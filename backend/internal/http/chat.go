package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/agreser-innovations/citadel-ia/backend/internal/auth"
	"github.com/agreser-innovations/citadel-ia/backend/internal/database"
	"github.com/agreser-innovations/citadel-ia/backend/pkg/problem"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const maxChatToolCalls = 3

var chatHTTPClient = &http.Client{Timeout: 90 * time.Second}

type chatMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	ToolName string `json:"tool_name,omitempty"`
}

type chatTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatContext struct {
	Role             string `json:"role"`
	EmpresaNome      string `json:"empresa_nome"`
	RestritoUnidades bool   `json:"restrito_unidades"`
}

type chatCoreRequest struct {
	Mensagens []chatMessage `json:"mensagens"`
	Tools     []chatTool    `json:"tools"`
	Contexto  chatContext   `json:"contexto"`
}

type chatCoreResponse struct {
	Acao     string         `json:"acao"`
	Resposta string         `json:"resposta"`
	ToolName string         `json:"tool_name"`
	ToolArgs map[string]any `json:"tool_args"`
	Tokens   int            `json:"tokens"`
	Provider string         `json:"provider"`
}

type chatHTTPInput struct {
	Mensagens []chatMessage `json:"mensagens"`
}

func chatTools() []chatTool {
	object := func(properties map[string]any) map[string]any {
		return map[string]any{
			"type":                 "object",
			"properties":           properties,
			"additionalProperties": false,
		}
	}
	return []chatTool{
		{
			Name:        "resumo_compliance",
			Description: "Consulta o resumo geral de compliance e conformidade da empresa, com colaboradores ativos e itens por status.",
			Parameters:  object(map[string]any{}),
		},
		{
			Name:        "pendencias_colaborador",
			Description: "Consulta pendências de compliance, documentos pendentes ou vencidos por colaborador; pode filtrar pelo nome.",
			Parameters: object(map[string]any{
				"nome": map[string]any{"type": "string", "description": "Nome completo ou parte do nome do colaborador"},
			}),
		},
		{
			Name:        "riscos_abertos",
			Description: "Consulta riscos trabalhistas e indicadores de risco que estão abertos na empresa.",
			Parameters:  object(map[string]any{}),
		},
		{
			Name:        "buscar_documentos",
			Description: "Busca documentos da empresa por palavras do conteúdo, tipo, título ou colaborador; sem termo, lista documentos recentes.",
			Parameters: object(map[string]any{
				"termo": map[string]any{"type": "string", "description": "Palavras para localizar no documento"},
			}),
		},
	}
}

// chat atende o widget sem confiar em ferramentas ou contexto enviados pelo navegador.
// O IA Core recebe apenas ferramentas de consulta autorizadas pelo backend.
func (s *Server) chat(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var in chatHTTPInput
	if !bindJSON(c, &in) {
		return
	}
	if len(in.Mensagens) == 0 || len(in.Mensagens) > 10 {
		problem.Write(c, problem.BadRequest("envie de 1 a 10 mensagens no histórico"))
		return
	}
	for i, msg := range in.Mensagens {
		if msg.Role != "user" && msg.Role != "assistant" {
			problem.Write(c, problem.BadRequest("o histórico aceita somente mensagens de usuário e assistente"))
			return
		}
		if len(msg.Content) > 4000 {
			problem.Write(c, problem.BadRequest("cada mensagem deve ter no máximo 4000 caracteres"))
			return
		}
		in.Mensagens[i].ToolName = ""
	}
	if in.Mensagens[len(in.Mensagens)-1].Role != "user" {
		problem.Write(c, problem.BadRequest("a última mensagem deve ser do usuário"))
		return
	}

	units, canReadHealth := s.userScope(c)
	if c.IsAborted() {
		return
	}
	cl := auth.ClaimsOf(c)
	companyName := ""
	if !s.tx(c, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(nullif(nome_fantasia,''),razao_social) FROM tenants WHERE id=$1`, tid(c)).Scan(&companyName)
	}) {
		return
	}

	coreReq := chatCoreRequest{
		Mensagens: in.Mensagens,
		Tools:     chatTools(),
		Contexto: chatContext{
			Role:             cl.Role,
			EmpresaNome:      companyName,
			RestritoUnidades: len(units) > 0,
		},
	}
	fontes := make([]string, 0, maxChatToolCalls)
	fontesVistas := make(map[string]bool)
	totalTokens := 0

	for chamadas := 0; chamadas <= maxChatToolCalls; chamadas++ {
		out, err := s.completeChat(c.Request.Context(), coreReq)
		if err != nil {
			slog.Error("falha ao completar chat", "request_id", c.GetString("request_id"), "erro", err)
			problem.Write(c, problem.New(http.StatusBadGateway, "ia-core-indisponivel", "não foi possível consultar o assistente agora"))
			return
		}
		if out.Tokens > 0 {
			totalTokens += out.Tokens
		}
		if out.Acao == "responder" {
			c.JSON(http.StatusOK, gin.H{"resposta": out.Resposta, "fontes": fontes, "tokens": totalTokens})
			return
		}
		if out.Acao != "chamar_ferramenta" || !knownChatTool(out.ToolName) {
			c.JSON(http.StatusOK, gin.H{
				"resposta": "Não consegui concluir essa consulta com as ferramentas de leitura disponíveis.",
				"fontes":   fontes,
				"tokens":   totalTokens,
			})
			return
		}
		if chamadas == maxChatToolCalls {
			break
		}

		result, source, err := s.runChatTool(c, out.ToolName, out.ToolArgs, units, canReadHealth)
		if err != nil {
			if p, ok := err.(*problem.Problem); ok {
				problem.Write(c, p)
				return
			}
			slog.Error("falha na consulta do chat", "request_id", c.GetString("request_id"), "ferramenta", out.ToolName, "erro", err)
			problem.Write(c, problem.Internal())
			return
		}
		if !fontesVistas[source] {
			fontesVistas[source] = true
			fontes = append(fontes, source)
		}
		content, err := json.Marshal(map[string]any{"fonte": source, "dados": json.RawMessage(result)})
		if err != nil {
			problem.Write(c, problem.Internal())
			return
		}
		coreReq.Mensagens = append(coreReq.Mensagens, chatMessage{
			Role:     "tool",
			ToolName: out.ToolName,
			Content:  string(content),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"resposta": "A consulta excedeu o limite de etapas. Tente fazer uma pergunta mais específica.",
		"fontes":   fontes,
		"tokens":   totalTokens,
	})
}

func (s *Server) completeChat(ctx context.Context, body chatCoreRequest) (chatCoreResponse, error) {
	var out chatCoreResponse
	data, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.Cfg.IACoreURL, "/")+"/chat/complete", bytes.NewReader(data))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-IACore-Key", s.Cfg.IACoreAPIKey)
	resp, err := chatHTTPClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, errors.New("IA Core retornou status " + resp.Status)
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return out, err
	}
	return out, nil
}

func knownChatTool(name string) bool {
	switch name {
	case "resumo_compliance", "pendencias_colaborador", "riscos_abertos", "buscar_documentos":
		return true
	default:
		return false
	}
}

func chatStringArg(args map[string]any, key string, maxLen int) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	for k := range args {
		if k != key {
			return "", errors.New("parâmetro desconhecido")
		}
	}
	v, ok := args[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || len(s) > maxLen {
		return "", errors.New("parâmetro inválido")
	}
	return strings.TrimSpace(s), nil
}

func (s *Server) runChatTool(c *gin.Context, name string, args map[string]any, units []string, canReadHealth bool) ([]byte, string, error) {
	ctx := c.Request.Context()
	var raw []byte
	var source string
	err := s.DB.Tenant(ctx, tid(c), func(tx pgx.Tx) error {
		var err error
		switch name {
		case "resumo_compliance":
			source = "Resumo de compliance"
			raw, err = database.JSON(ctx, tx, `SELECT json_build_object(
			  'colaboradores_ativos', (SELECT count(*) FROM employees e WHERE e.status<>'DESLIGADO' AND ($1::uuid[]='{}' OR e.unit_id=ANY($1))),
			  'itens_por_status', coalesce((SELECT json_agg(t ORDER BY t.status) FROM (
			    SELECT ci.status, count(*) AS total FROM compliance_items ci
			    JOIN requirements req ON req.id=ci.requirement_id
			    JOIN document_types dt ON dt.id=req.document_type_id
			    LEFT JOIN employees e ON e.id=ci.employee_id
			    WHERE (NOT dt.sensivel_saude OR $2)
			      AND ((ci.employee_id IS NOT NULL AND e.status<>'DESLIGADO' AND ($1::uuid[]='{}' OR e.unit_id=ANY($1)))
			        OR (ci.employee_id IS NULL AND $1::uuid[]='{}'))
			    GROUP BY ci.status
			  ) t), '[]'::json))`, units, canReadHealth)
		case "pendencias_colaborador":
			nome, argErr := chatStringArg(args, "nome", 120)
			if argErr != nil {
				return problem.BadRequest("filtro de colaborador inválido")
			}
			source = "Pendências por colaborador"
			raw, err = database.JSON(ctx, tx, `SELECT coalesce(json_agg(t ORDER BY t.funcionario, t.tipo),'[]'::json) FROM (
			  SELECT e.nome AS funcionario, dt.nome AS tipo, ci.referencia, ci.status, ci.vence_em
			  FROM compliance_items ci
			  JOIN employees e ON e.id=ci.employee_id
			  JOIN requirements req ON req.id=ci.requirement_id
			  JOIN document_types dt ON dt.id=req.document_type_id
			  WHERE ci.status IN ('PENDENTE','VENCIDO','A_VENCER') AND e.status<>'DESLIGADO'
			    AND (NOT dt.sensivel_saude OR $3)
			    AND ($1::uuid[]='{}' OR e.unit_id=ANY($1))
			    AND ($2='' OR strpos(f_unaccent(lower(e.nome)),f_unaccent(lower($2)))>0)
			  ORDER BY e.nome, dt.nome LIMIT 50
			) t`, units, nome, canReadHealth)
		case "riscos_abertos":
			source = "Riscos abertos"
			raw, err = database.JSON(ctx, tx, `SELECT coalesce(json_agg(t ORDER BY t.prioridade, t.funcionario),'[]'::json) FROM (
			  SELECT r.severidade AS prioridade, r.categoria, r.codigo, r.titulo, r.descricao, r.base_legal,
			    e.nome AS funcionario, r.criado_em
			  FROM risk_indicators r
			  LEFT JOIN employees e ON e.id=r.employee_id
			  WHERE r.status='ABERTO'
			    AND ((r.employee_id IS NOT NULL AND e.status<>'DESLIGADO' AND ($1::uuid[]='{}' OR e.unit_id=ANY($1))) OR (r.employee_id IS NULL AND $1::uuid[]='{}'))
			    AND ($2 OR r.document_id IS NULL OR EXISTS (SELECT 1 FROM documents d WHERE d.id=r.document_id AND NOT d.sensivel_saude))
			  ORDER BY CASE r.severidade WHEN 'ALTO' THEN 0 WHEN 'MEDIO' THEN 1 ELSE 2 END, r.criado_em DESC LIMIT 50
			) t`, units, canReadHealth)
		case "buscar_documentos":
			termo, argErr := chatStringArg(args, "termo", 160)
			if argErr != nil {
				return problem.BadRequest("termo de busca inválido")
			}
			source = "Busca de documentos"
			raw, err = database.JSON(ctx, tx, `SELECT coalesce(json_agg(t ORDER BY t.rank DESC, t.criado_em DESC),'[]'::json) FROM (
			  SELECT d.document_type, dt.nome AS tipo_nome, d.titulo, d.status, d.competencia, d.data_documento,
			    e.nome AS funcionario, d.criado_em,
			    CASE WHEN $1='' OR NOT (d.tsv @@ q.query) THEN '' ELSE ts_headline('portuguese', f_unaccent(d.texto_anonimizado), q.query,
			      'MaxWords=24, MinWords=8, StartSel=<<, StopSel=>>, MaxFragments=1') END AS trecho,
			    CASE WHEN $1='' OR NOT (d.tsv @@ q.query) THEN 0 ELSE ts_rank(d.tsv, q.query) END AS rank
			  FROM documents d
			  JOIN document_types dt ON dt.codigo=d.document_type
			  LEFT JOIN employees e ON e.id=d.employee_id
			  CROSS JOIN (SELECT websearch_to_tsquery('portuguese', f_unaccent($1)) AS query) q
			  WHERE d.status NOT IN ('REJEITADO','SUBSTITUIDO')
			    AND (NOT d.sensivel_saude OR $3)
			    AND ($2::uuid[]='{}' OR d.unit_id=ANY($2) OR e.unit_id=ANY($2))
			    AND ($1='' OR d.tsv @@ q.query OR strpos(
			      f_unaccent(lower(coalesce(d.titulo,'')||' '||dt.nome||' '||coalesce(e.nome,'')||' '||coalesce(d.nome_arquivo,''))),
			      f_unaccent(lower($1)))>0)
			  ORDER BY rank DESC, d.criado_em DESC LIMIT 20
			) t`, termo, units, canReadHealth)
		default:
			return errors.New("ferramenta de chat não permitida")
		}
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return raw, source, nil
}
