# Recortes, assinaturas e indicadores por lote

A migração `000011_recortes` acrescenta folha de origem, número da região, caixa do recorte e orientação às páginas. O fatiamento mantém o arquivo original e gera imagens próprias para cada região. O visualizador identifica a folha e o recorte. As cópias PDF usam essas imagens e as coordenadas de OCR recalculadas.

O recorte é local, sem chamada adicional ao modelo: procura separações largas sem tinta e exige um cabeçalho documental reconhecível em cada região. Suporta documentos empilhados ou lado a lado, até quatro regiões por folha. Frente e verso passam separadamente pela leitura e voltam a formar um documento quando as regras de continuação, identificação e titular permitem. CPF ou identificador divergente impede a união. Layouts sobrepostos, sem separação clara, com cabeçalho desconhecido ou ilegível podem permanecer inteiros; a análise multimodal pode sinalizar múltiplos documentos para revisão. Não é uma detecção universal de contornos.

A cota mensal conta folhas originais. `batches.total_paginas` continua representando as regiões do processamento; `total_folhas` é a quantidade antes do recorte. As médias do painel deixam explícito qual denominador usam.

## Assinaturas

O rótulo impresso “Assinatura” não comprova assinatura. O checklist usa o campo booleano `assinatura_detectada`: `true` indica presença detectada, `false` indica ausência e `null` significa não verificada. Tipos com assinatura obrigatória passam pela visão. Eles e formulários como o AR usam uma cópia com limite de pelo menos 1800 pixels no maior lado (sem ampliar o original). Uma assinatura identificada no verso vale para o documento agrupado; ausência só é concluída se todas as páginas tiverem sido verificadas. Sem provedor visual, a assinatura segue para revisão. A presença detectada não verifica autoria, validade criptográfica ou todas as assinaturas exigidas.

O revisor pode corrigir o campo no formulário. Checklist, motivo de revisão e indicador de ausência são atualizados em conjunto; as demais regras permanecem. O limiar de confiança configurado no lote também é usado pelo checklist.

## Consumo e custo

`GET /api/v1/admin/ia/metrics?dias=30&tenant=&offset=0` é exclusivo do Master e retorna apenas indicadores e metadados. Período de 1 a 365 dias, agrupado pela data de recebimento do lote, páginas de 50 lotes. Resumo e médias consideram todo o filtro, não só a página da tabela. O painel aparece em Visão geral, Filas e jobs e Uso da IA por empresa.

A fonte do consumo são os checkpoints das páginas, não documentos que podem ser divididos, unidos ou descartados. Somam-se chamadas de texto, visão e modelo reserva que retornaram uso. Reutilização de cache zera tokens, chamadas e custo do novo lote. Páginas em branco que consumiram modelo continuam contando. Falhas sem resposta de uso, respostas JSON inválidas e chamadas interrompidas antes do checkpoint podem não estar contabilizadas; os valores não substituem a fatura do provedor.

Configure `IA_PRICES_BRL_PER_MILLION` no ambiente do IA Core: JSON cujas chaves são os nomes exatos dos modelos e cujos valores contêm `input`, `output` e, opcionalmente, `cached_input`. As tarifas são em BRL por **milhão** de tokens. Exemplo de estrutura com valores **fictícios**, sem relação com preços de provedor:

```dotenv
IA_PRICES_BRL_PER_MILLION={"modelo-exemplo":{"input":1,"output":4,"cached_input":0.5}}
```

O cálculo é `((entrada − cache) × input + cache × cached_input + saída × output) / 1.000.000`. Na ausência de tarifa de cache, usa-se `input`. A tarifa aplicada fica registrada junto à chamada. Tokens de raciocínio informados no total do Gemini são incluídos na saída. Modelo sem preço deixa o custo total sem valor; o painel exibe o custo parcial conhecido e a quantidade de regiões sem tarifa. Nenhuma tarifa oficial é presumida. O custo é mantido com casas decimais, com arredondamento para centavos apenas na soma contábil mensal do lote. A alteração de tarifas vale para novas análises, sem recalcular custos históricos.

- **Tokens por folha**: tokens registrados / folhas físicas recebidas, incluindo brancas e pendentes.
- **Tokens por região**: tokens registrados / regiões com análise concluída, incluindo cache.
- **Custo por folha**: custo estimado / folhas físicas, disponível quando todas as chamadas têm tarifa.

## Qualidade

- **Confiança média**: média da confiança de páginas não brancas, fornecida pelo motor; não é acurácia.
- **Acurácia da classificação**: documentos revisados cujo tipo inicial coincide com o tipo após revisão / documentos com revisão concluída e histórico comparável.
- **Leitura sem correção**: fração dessa amostra com tipo, campos, competência e páginas iguais, sem divisão, união ou transcrição manual. Mede concordância de dados estruturados, não erro por caractere do OCR.
- **Cobertura**: quantidade revisada / documentos de origem IA; documentos criados por divisão manual são excluídos para não inflar a amostra.

Múltiplas revisões do mesmo documento contam uma vez, comparando o primeiro estado com o último. Rejeitados, documentos manuais e ainda não revisados não entram na amostra de acurácia. Sem amostra, o resultado é nulo, nunca 100%. A revisão costuma concentrar documentos difíceis e a amostra não é representativa de todo o lote. O painel mostra também assinaturas não verificadas, regiões ilegíveis, motivos de revisão, múltiplos documentos pendentes, falhas, recortes, tempo e consumo por modelo.

As mudanças de reconhecimento e recorte valem para novos processamentos; documentos já concluídos não são alterados automaticamente. Métricas históricas usam o que foi registrado: lotes antigos podem ter tokens sem detalhamento de modelo ou tarifa.

## Validação

- `cd ia-core && ../.venv/bin/python -m pytest tests -q`
- `cd frontend && npm test && npm run lint && npm run build`
- `cd backend && go test -race ./...`
- Para os testes SQL: aplicar todas as migrações em um banco descartável e executar `METRICS_TEST_DATABASE_URL=... go test ./internal/batches ./internal/http -run 'TestPageCache|TestIAMetrics' -v`. Os testes usam transação com rollback e cobrem isolamento por empresa, motor e origem do texto, amostra revisada, custos pequenos, ausência de tarifa, cache, médias por folha/região e tempos por camada.

## Desempenho das camadas 0–3

A migração `000012_cache_motor` registra a origem do texto e cria o índice de consulta do cache. Aplique as migrações antes de reiniciar API, worker e IA Core; publique também o frontend atualizado. Não há troca automática dos modelos configurados.

- **Camada 0:** consulta apenas os hashes pendentes do lote por empresa, motor, origem do texto e tipo de lote, com índice e busca da análise mais recente. Dentro de uma execução, até 128 páginas idênticas podem compartilhar resultado ou chamada em andamento; a chave inclui o texto e o tipo da página anterior. Cópias têm zero tokens/custo e não herdam tempos da análise original. Reutilização entre lotes conserva a interpretação anterior; casos dependentes da sequência precisam de revisão da separação.
- **Camada 1:** pistas e expressões regulares são preparadas uma vez e reutilizadas; novas pistas continuam sendo reconhecidas. A etapa local lê a página uma única vez e só constrói prompts quando precisa subir de camada. No fatiamento, imagens e miniaturas de folhas sem recorte são copiadas sem nova codificação.
- **Camada 2:** schema e sua representação JSON são reutilizados. Instruções estáveis precedem o contexto variável, favorecendo o cache de prefixo do provedor quando aplicável. Espaços redundantes são compactados; textos acima de `IA_TEXT_MAX_CHARS` seguem para visão, preservando o conteúdo em vez de cortar o final.
- **Camada 3:** texto nativo legível e com boa classificação local acompanha a imagem para evitar retranscrição do texto impresso. A visão ainda examina toda a imagem e transcreve complementos manuscritos; divergências devem produzir transcrição completa. OCR prévio não recebe esse tratamento. A reserva recebe mais resolução nos casos difíceis e sua resposta só substitui a primeira se as verificações de legibilidade, tipo, assinatura e confiança não a classificarem como inferior. Ambas as chamadas continuam contabilizadas. Essa escolha é heurística, não uma prova de correção.

Downloads e callbacks usam conexões HTTP persistentes. A concorrência de modelos continua limitada por `IA_LLM_CONCURRENCY`; mais concorrência não foi presumida como ganho gratuito.

O painel Master agrega **pela camada final**: tokens por região, tempo local, espera na fila dos modelos, execução dos modelos, média, P95 e tamanho da amostra. `desempenho.etapas` registra duração/status/modelo de cada tentativa. O tempo total soma download, etapa local, fila e modelos; não inclui upload, fatiamento, callbacks, agrupamento ou espera de uma cópia pela chamada compartilhada. Não representa a duração de ponta a ponta do lote. Dados históricos sem tempos ficam nulos.

Microbenchmark sintético local, mediana de cinco rodadas na mesma máquina, antes/depois da alteração:

| Etapa | Antes (ms/operação) | Depois (ms/operação) |
| --- | ---: | ---: |
| Classificação por regras | 0,699880 | 0,270405 |
| Schema + instrução | 1,023685 | 0,000228 |
| Análise local com texto nativo | 1,012337 | 0,586083 |

O classificador usou cerca de 61% menos tempo nessa amostra. O schema já aquecido praticamente elimina o trabalho de construção. São medidas internas, sem rede, OCR de imagem ou API paga; não demonstram ganho no lote inteiro nem melhora de acurácia. Para repetir a medição da versão atual: `.venv/bin/python ia-core/tools/benchmark_camadas.py`, na raiz. A validação de reconhecimento real deve comparar lotes representativos revisados, custo, taxa de escalonamento e duração antes/depois.

Referências consultadas: [cache de prompts da OpenAI](https://developers.openai.com/api/docs/guides/prompt-caching) e [leitura de imagens e detalhe visual](https://developers.openai.com/api/docs/guides/images-vision). Cache depende de prefixos compatíveis e das condições do provedor; imagens com texto pequeno precisam manter detalhe suficiente.
