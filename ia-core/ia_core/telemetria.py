"""Consumo retornado pelo provedor; preços configuráveis em BRL por milhão de tokens."""
import json
import math
import os


def chamada(modelo, total, entrada=0, saida=0, cache=0):
    total, entrada, saida, cache = (max(0, int(v or 0)) for v in (total, entrada, saida, cache))
    # O detalhamento de uso independe da configuração de preços.
    saida = max(saida, total - entrada)
    cache = min(cache, entrada)
    custo = None
    tarifa = None
    try:
        tarifa = json.loads(os.environ.get("IA_PRICES_BRL_PER_MILLION") or "{}").get(modelo)
        if tarifa:
            valores = [float(tarifa[k]) for k in ("input", "output")]
            cached = float(tarifa.get("cached_input", valores[0]))
            if all(math.isfinite(v) and v >= 0 for v in [*valores, cached]):
                custo = ((entrada-cache)*valores[0] + cache*cached + saida*valores[1])/1_000_000
    except (ValueError, TypeError, KeyError, AttributeError):
        tarifa = None
    return {"modelo": modelo, "tokens": total, "entrada": entrada, "saida": saida, "cache": cache,
            "custo_estimado_brl": custo, "tarifa_brl_milhao": tarifa}


def resumo(chamadas):
    custos = [c.get("custo_estimado_brl") for c in chamadas]
    return {"chamadas": chamadas, "tokens_entrada": sum(c["entrada"] for c in chamadas),
            "tokens_saida": sum(c["saida"] for c in chamadas), "tokens_cache": sum(c["cache"] for c in chamadas),
            "custo_estimado_brl": sum(custos) if all(c is not None for c in custos) else None,
            "custo_conhecido_brl": sum(c for c in custos if c is not None),
            "chamadas_sem_preco": sum(c is None for c in custos)}
