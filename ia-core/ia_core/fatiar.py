"""Fatiamento em streaming: PDF/TIFF/imagem/ZIP -> PNG por página + miniatura WebP + texto/palavras/caixas de PII."""
import io
import json
import os
import re
import tempfile
import zipfile
from typing import Dict, List

import fitz  # PyMuPDF
from PIL import Image, ImageSequence

from .orientacao import endireitar
from .recorte import separar, reposicionar
from .anonimizacao import PADROES, GRUPOS, caixas_de_palavras

DPI = 150
THUMB_W = 360


def _rel(r, w, h):
    return [round(r.x0 / w, 5), round(r.y0 / h, 5), round(r.x1 / w, 5), round(r.y1 / h, 5)]


def _caixas_pii(page: "fitz.Page", texto: str) -> List[list]:
    w, h = page.rect.width, page.rect.height
    achados = []
    for tipo, rx in PADROES:
        for m in rx.finditer(texto):
            achados.append((tipo, m.group(0)))
    for tipo, rx in GRUPOS:
        for m in rx.finditer(texto):
            achados.append((tipo, m.group(1).strip()))
    caixas, vistos = [], set()
    for tipo, s in achados:
        if (tipo, s) in vistos:
            continue
        vistos.add((tipo, s))
        for r in page.search_for(s):
            caixas.append(_rel(r, w, h) + [tipo])
    return caixas


def _pagina_pdf(page: "fitz.Page", numero: int, saida: zipfile.ZipFile) -> dict:
    pix = page.get_pixmap(dpi=DPI)
    png = pix.tobytes("png")
    texto = (page.get_text("text") or "").strip()
    w, h = page.rect.width, page.rect.height
    palavras = [_rel(fitz.Rect(x0, y0, x1, y1), w, h) + [t] for x0, y0, x1, y1, t, *_ in page.get_text("words")]
    if not texto:  # página escaneada (só imagem): se veio de cabeça para baixo ou de lado, a imagem guardada é girada
        png, graus = endireitar(png)
        if graus:
            im = Image.open(io.BytesIO(png))
            return _gravar(saida, numero, png, im.width, im.height, "", [], [], "pdf", graus)
    return _gravar(saida, numero, png, pix.width, pix.height, texto, palavras, _caixas_pii(page, texto) if texto else [], "pdf")


def _gravar(saida, numero, png: bytes, w: int, h: int, texto: str, palavras, caixas, captura: str, rotacao_corrigida: int = 0) -> dict:
    saida.writestr(f"pages/{numero:04d}.png", png)
    im = Image.open(io.BytesIO(png)).convert("RGB")
    im.thumbnail((THUMB_W, THUMB_W * 2))
    b = io.BytesIO()
    im.save(b, "WEBP", quality=70)
    saida.writestr(f"thumbs/{numero:04d}.webp", b.getvalue())
    return {"numero": numero, "largura": w, "altura": h, "texto_embutido": texto, "captura": captura, "palavras": palavras, "caixas": caixas,
            "rotacao_corrigida": rotacao_corrigida, "texto_nativo": captura == "pdf" and bool(texto)}


def _pagina_imagem(img: Image.Image, numero: int, saida: zipfile.ZipFile) -> dict:
    rgb = img.convert("RGB")
    b = io.BytesIO()
    rgb.save(b, "PNG")
    png, graus = endireitar(b.getvalue())  # foto/scan de cabeça para baixo ou de lado: guarda a imagem na orientação correta
    if graus:
        rgb = Image.open(io.BytesIO(png)).convert("RGB")
    texto, palavras, caixas = "", [], []
    try:  # OCR posicionado se o Tesseract estiver disponível
        doc = fitz.open()
        pg = doc.new_page(width=rgb.width, height=rgb.height)
        pg.insert_image(pg.rect, stream=png)
        tp = pg.get_textpage_ocr(language="por", dpi=DPI, full=True)
        texto = (pg.get_text("text", textpage=tp) or "").strip()
        w, h = pg.rect.width, pg.rect.height
        palavras = [_rel(fitz.Rect(x0, y0, x1, y1), w, h) + [t] for x0, y0, x1, y1, t, *_ in pg.get_text("words", textpage=tp)]
        caixas = caixas_de_palavras(palavras)
    except Exception:
        pass
    return _gravar(saida, numero, png, rgb.width, rgb.height, texto, palavras, caixas, "foto" if max(rgb.size) < 2500 else "scanner", graus)


def _fatiar_original(caminho: str, nome: str, destino_zip: str) -> int:
    """Escreve o ZIP (manifest.json + pages + thumbs) em disco, página a página (sem carregar o arquivo inteiro)."""
    ext = os.path.splitext(nome.lower())[1]
    paginas: List[dict] = []
    with zipfile.ZipFile(destino_zip, "w", zipfile.ZIP_STORED) as z:
        if ext == ".pdf":
            with fitz.open(caminho) as doc:
                for i in range(len(doc)):
                    paginas.append(_pagina_pdf(doc.load_page(i), i + 1, z))
        elif ext == ".zip":
            with zipfile.ZipFile(caminho) as zin:
                nomes = sorted(n for n in zin.namelist() if n.lower().endswith((".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff")) and not n.startswith("__MACOSX"))
                n = 0
                for nm in nomes:
                    with zin.open(nm) as f:
                        im = Image.open(io.BytesIO(f.read()))
                        for fr in ImageSequence.Iterator(im):
                            n += 1
                            paginas.append(_pagina_imagem(fr.copy(), n, z))
        else:  # TIFF multipágina, JPG, PNG, HEIC (se pillow-heif estiver instalado)
            if ext == ".heic":
                try:
                    import pillow_heif
                    pillow_heif.register_heif_opener()
                except Exception:
                    raise ValueError("HEIC requer pillow-heif no IA Core")
            with Image.open(caminho) as im:
                for i, fr in enumerate(ImageSequence.Iterator(im)):
                    paginas.append(_pagina_imagem(fr.copy(), i + 1, z))
        z.writestr("manifest.json", json.dumps({"paginas": paginas}, ensure_ascii=False))
    return len(paginas)


def fatiar(caminho: str, nome: str, destino_zip: str) -> int:
    with tempfile.TemporaryDirectory() as tmp:
        bruto = os.path.join(tmp, "folhas.zip")
        total = _fatiar_original(caminho, nome, bruto)
        paginas = []
        with zipfile.ZipFile(bruto) as src, zipfile.ZipFile(destino_zip, "w", zipfile.ZIP_STORED) as out:
            originais = json.loads(src.read("manifest.json"))["paginas"]
            for pg in originais:
                png = src.read(f"pages/{pg['numero']:04d}.png")
                boxes = separar(png, pg["palavras"])
                im = None
                for indice, box in enumerate(boxes, 1):
                    numero = len(paginas)+1
                    if len(boxes) == 1:
                        out.writestr(f"pages/{numero:04d}.png", png)
                        out.writestr(f"thumbs/{numero:04d}.webp", src.read(f"thumbs/{pg['numero']:04d}.webp"))
                        nova = {**pg, "numero": numero}
                    else:
                        if im is None:
                            im = Image.open(io.BytesIO(png))
                        crop = im.crop((round(box[0]*im.width), round(box[1]*im.height), round(box[2]*im.width), round(box[3]*im.height)))
                        b = io.BytesIO(); crop.save(b, "PNG")
                        palavras = reposicionar(pg["palavras"], box)
                        if palavras:
                            # Reconstrói linhas mantendo a ordem de leitura e a posição de cada palavra.
                            linhas = []
                            for palavra in sorted(palavras, key=lambda p: (p[1], p[0])):
                                if not linhas or abs(palavra[1]-linhas[-1][0]) > 0.008:
                                    linhas.append((palavra[1], [palavra]))
                                else:
                                    linhas[-1][1].append(palavra)
                            texto = "\n".join(" ".join(str(p[4]) for p in sorted(l[1], key=lambda p:p[0])) for l in linhas)
                            nova = _gravar(out, numero, b.getvalue(), crop.width, crop.height, texto, palavras, caixas_de_palavras(palavras), pg["captura"], pg["rotacao_corrigida"])
                        else:
                            nova = _pagina_imagem(crop, numero, out)
                    nova.update(pagina_origem=pg["numero"], regiao=indice, recorte=box,
                                rotacao_corrigida=pg["rotacao_corrigida"])
                    paginas.append(nova)
            out.writestr("manifest.json", json.dumps({"paginas": paginas, "total_folhas": total}, ensure_ascii=False))
        return len(paginas)
