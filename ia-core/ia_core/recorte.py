"""Recorte conservador: separação ampla sem tinta + cabeçalhos documentais em ambos os lados.

Não recorta tabelas/colunas pelo espaço sozinho. Regiões ambíguas ficam inteiras para a revisão.
As caixas são relativas à imagem orientada; o arquivo original permanece no armazenamento.
"""
import io

from PIL import Image

from .catalogo import COMPLEMENTOS, TIPOS, norm
from .orientacao import ocr_texto


def _cabecalho(texto):
    linhas = [norm(l) for l in texto.splitlines() if l.strip()][:4]
    for linha in linhas:
        for tipo, dados in list(TIPOS.items()):
            if tipo != "OUTRO" and any(len(p) >= 8 and (linha == p or linha.startswith(p + " ")) for p in dados["pistas"]):
                return tipo
        for frase, tipo in COMPLEMENTOS.items():
            if linha.startswith(frase):
                return tipo
    return None


def _candidatos(im):
    small = im.convert("L")
    small.thumbnail((600, 600))
    w, h = small.size
    encontrados = []
    for eixo, tamanho in ((0, w), (1, h)):
        inicio = None
        for i in range(tamanho + 1):
            vazio = False
            if i < tamanho:
                faixa = small.crop((i, 0, i+1, h) if eixo == 0 else (0, i, w, i+1))
                hist = faixa.histogram()
                vazio = sum(hist[:210]) <= (h if eixo == 0 else w) * 0.002
            if vazio and inicio is None:
                inicio = i
            if not vazio and inicio is not None:
                centro = (inicio+i)/2/tamanho
                if i-inicio >= tamanho*0.04 and 0.18 < centro < 0.82:
                    encontrados.append((i-inicio, eixo, centro))
                inicio = None
    return sorted(encontrados, reverse=True)


def separar(png, palavras):
    """Lista de caixas [x0,y0,x1,y1]; até quatro regiões, sem perder tinta ou margens."""
    im = Image.open(io.BytesIO(png)).convert("RGB")

    def texto_caixa(box):
        crop = im.crop((round(box[0]*im.width), round(box[1]*im.height), round(box[2]*im.width), round(box[3]*im.height)))
        b = io.BytesIO(); crop.save(b, "PNG")
        try:
            return ocr_texto(b.getvalue())
        except Exception:
            return ""

    # Palavras nativas precisam ser reunidas em linhas para reconhecer o título, sem misturar colunas.
    def titulo(box):
        if palavras:
            ps = [p for p in palavras if box[0] <= (p[0]+p[2])/2 <= box[2] and box[1] <= (p[1]+p[3])/2 <= box[3]]
            linhas = []
            for p in sorted(ps, key=lambda p: (p[1], p[0])):
                if not linhas or abs(p[1]-linhas[-1][0]) > 0.008:
                    linhas.append((p[1], [p]))
                else:
                    linhas[-1][1].append(p)
            texto = "\n".join(" ".join(str(p[4]) for p in sorted(l[1], key=lambda p: p[0])) for l in linhas)
        else:
            texto = texto_caixa(box)
        return _cabecalho(texto)

    def dividir(box, nivel):
        if nivel >= 2:
            return [box]
        rec = im.crop((round(box[0]*im.width), round(box[1]*im.height), round(box[2]*im.width), round(box[3]*im.height)))
        for _, eixo, pos in _candidatos(rec)[:4]:
            corte = box[eixo] + pos*(box[eixo+2]-box[eixo])
            a, b = list(box), list(box)
            a[eixo+2], b[eixo] = corte, corte
            if titulo(a) and titulo(b):
                return dividir(a, nivel+1) + dividir(b, nivel+1)
        return [box]

    return dividir([0.0, 0.0, 1.0, 1.0], 0)


def reposicionar(palavras, box):
    x, y, x1, y1 = box
    return [[max(0, (p[0]-x)/(x1-x)), max(0, (p[1]-y)/(y1-y)),
             min(1, (p[2]-x)/(x1-x)), min(1, (p[3]-y)/(y1-y)), *p[4:]]
            for p in palavras if x <= (p[0]+p[2])/2 <= x1 and y <= (p[1]+p[3])/2 <= y1]
