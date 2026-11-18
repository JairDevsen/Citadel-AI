"""Impressão digital do MOTOR de análise: provedor, modelos, regras e código que produzem a análise de uma página.

A Camada 0 (cache por SHA-256 da página) só pode reaproveitar uma análise feita pelo MESMO motor. Se o catálogo, as regras de
extração, o prompt, o modelo ou o código mudam, a impressão muda e as análises antigas deixam de valer: a página é analisada de
novo em vez de herdar um resultado produzido por regras que já não existem."""
import hashlib
import os

# arquivos cujo conteúdo define o resultado da análise de uma página
_ARQUIVOS = ("catalogo.py", "extracao.py", "validacoes.py", "checklist.py", "providers.py", "orientacao.py", "qualidade_img.py",
             "paginacao.py", "anonimizacao.py", "agrupar.py", "schemas.py", "roteador.py", "telemetria.py", "recorte.py", "fatiar.py", "main.py")
_AQUI = os.path.dirname(os.path.abspath(__file__))


def _codigo() -> str:
    h = hashlib.sha256()
    for nome in _ARQUIVOS:
        with open(os.path.join(_AQUI, nome), "rb") as f:
            h.update(nome.encode() + b"\0" + f.read() + b"\0")
    return h.hexdigest()


def impressao_motor(provider) -> str:
    """'provedor:modelo|código' — o que tiver de mudar para uma análise antiga deixar de valer."""
    ajustes = "|".join(str(getattr(provider, a, "")) for a in ("nome", "model", "fallback", "lite"))
    ajustes += "|" + "|".join(os.environ.get(k, "") for k in (
        "IA_IMAGE_MAX_PX", "GEMINI_IMAGE_MAX_PX", "IA_TEXT_MAX_CHARS", "IA_CONF_TIPO_NOVO",
        "IA_NITIDEZ_MIN", "IA_TINTA_MIN", "IA_TINTA_MARCA", "OPENAI_REASONING_EFFORT", "OPENAI_BASE_URL"))
    return f"{getattr(provider, 'nome', 'motor')}|{hashlib.sha256((ajustes + _codigo()).encode()).hexdigest()[:16]}"
