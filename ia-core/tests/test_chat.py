from ia_core.chat import ChatIn, FerramentaDef, MensagemChat, completar_chat
from ia_core.providers import LocalProvider


def test_local_mapeia_consulta_de_lotes_para_ferramenta_disponivel():
    body = ChatIn(
        mensagens=[MensagemChat(role="user", content="Quais lotes estão processando?")],
        tools=[
            FerramentaDef(
                name="consultar_lotes",
                description="Consulta o status dos lotes",
                parameters={"type": "object", "properties": {}, "additionalProperties": False},
            )
        ],
    )

    result = completar_chat(LocalProvider(), body)

    assert result.acao == "chamar_ferramenta"
    assert result.tool_name == "consultar_lotes"
    assert result.tool_args == {}


def test_local_nao_chama_ferramenta_de_escrita():
    body = ChatIn(
        mensagens=[MensagemChat(role="user", content="Aprove este documento")],
        tools=[FerramentaDef(name="aprovar_documento", description="Aprova documento")],
    )

    result = completar_chat(LocalProvider(), body)

    assert result.acao == "responder"
    assert "modo consultivo" in result.resposta


def test_local_nao_emite_chamada_com_parametros_obrigatorios_ausentes():
    body = ChatIn(
        mensagens=[MensagemChat(role="user", content="Consulte os lotes")],
        tools=[
            FerramentaDef(
                name="consultar_lotes",
                description="Consulta lotes",
                parameters={
                    "type": "object",
                    "properties": {"status": {"type": "string"}},
                    "required": ["status"],
                },
            )
        ],
    )

    result = completar_chat(LocalProvider(), body)

    assert result.acao == "responder"
    assert "parâmetros adicionais" in result.resposta


def test_local_formata_resultado_da_ferramenta():
    body = ChatIn(
        mensagens=[
            MensagemChat(role="tool", tool_name="consultar_lotes", content='{"status":"PROCESSANDO"}')
        ]
    )

    result = completar_chat(LocalProvider(), body)

    assert result.acao == "responder"
    assert "consultar_lotes" in result.resposta
    assert '"PROCESSANDO"' in result.resposta


def test_historico_e_mensagens_sao_limitados():
    from ia_core.chat import _historico

    body = ChatIn(
        mensagens=[MensagemChat(role="user", content=str(i) * 5000) for i in range(12)]
    )

    historico = _historico(body)

    assert len(historico) == 10
    assert all(len(mensagem["content"]) == 4000 for mensagem in historico)
