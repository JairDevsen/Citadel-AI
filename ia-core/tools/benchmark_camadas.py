"""Microbenchmark local, sem API ou documentos reais.

Execute da raiz: .venv/bin/python ia-core/tools/benchmark_camadas.py
Os resultados medem CPU local; não estimam latência do provedor ou acurácia.
"""
import json
from pathlib import Path
import statistics
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from ia_core.catalogo import classificar
from ia_core.providers import LocalProvider, OpenAIProvider

TEXTO = (
    "RECIBO DE PAGAMENTO SALARIO BASE LIQUIDO A RECEBER INSS "
    "CPF: 529.982.247-25 Competencia: 04/2026 Nome: Maria da Silva "
) * 5


def medir(fn, repeticoes):
    amostras = []
    for _ in range(5):
        inicio = time.perf_counter()
        for _ in range(repeticoes):
            fn()
        amostras.append((time.perf_counter() - inicio) * 1000 / repeticoes)
    return round(statistics.median(amostras), 6)


if __name__ == "__main__":
    # Sem inicialização de SDK/chaves: mede somente a montagem local da instrução.
    provider = OpenAIProvider.__new__(OpenAIProvider)
    print(json.dumps({
        "classificar_ms": medir(lambda: classificar(TEXTO), 200),
        "schema_instrucao_ms": medir(lambda: provider._instrucao("teste"), 30),
        "local_ms": medir(lambda: LocalProvider().analisar(b"", 1, None, TEXTO, "MISTO"), 100),
    }, indent=2))
