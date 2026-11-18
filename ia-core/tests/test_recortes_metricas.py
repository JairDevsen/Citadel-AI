import io
import json
import zipfile

import fitz
import pytest

from ia_core import agrupar, checklist, fatiar
from ia_core.providers import LLMProvider, CAMADA_2, CAMADA_3, _Uso, _soma
from ia_core.roteador import decidir
from ia_core.telemetria import chamada, resumo
from ia_core.validacoes import validar


def test_assinatura_visual_prevalece_sobre_rotulo_impresso():
    for valor, esperado in [(True, "OK"), (False, "FALHA"), (None, "REVISAO_HUMANA")]:
        campos = {"assinatura_detectada": valor}
        ck = checklist.montar("FICHA_EPI", "Assinatura do empregado: ______", campos, .95)
        assert next(c["status"] for c in ck if c["regra"] == "Assinatura identificada") == esperado
        _, ncs = validar("FICHA_EPI", "Assinatura do empregado: ______", campos)
        assert any(c["codigo"] == "DOCUMENTO_SEM_ASSINATURA" for c in ncs) == (valor is False)
    p = LLMProvider()
    d = {"tipo": "FICHA_EPI", "tem_assinatura": True}
    assert p._normalizar(d, None, CAMADA_3)["campos"]["assinatura_detectada"] is True
    assert "assinatura_detectada" not in p._normalizar(d, None, CAMADA_2)["campos"]
    assert decidir({"tipo": "CONTRATO_TRABALHO"}, "", True, []).camada == 3


def test_custo_entrada_saida_cache_e_tentativa_descartada(monkeypatch):
    monkeypatch.setenv("IA_PRICES_BRL_PER_MILLION", json.dumps({"texto":{"input":1,"output":4},"visao":{"input":2,"output":8,"cached_input":.5}}))
    a = chamada("texto",300,200,100)
    b = chamada("visao",1000,800,200,100)
    uso = _soma(_Uso(300,[a]),_Uso(1000,[b]))
    assert uso.total_token_count == 1300
    assert resumo(uso.chamadas)["custo_estimado_brl"] == pytest.approx(.0006+.00305)
    assert resumo([a,chamada("sem-tarifa",100,50,50)])["custo_estimado_brl"] is None
    assert resumo([])["custo_estimado_brl"] == 0
    monkeypatch.setenv("IA_PRICES_BRL_PER_MILLION", '{"x":{"input":-1,"output":2}}')
    assert chamada("x",100,50,50)["custo_estimado_brl"] is None


def _pagina(tipo, texto, inicio=True, assinatura=None):
    return dict(tipo=tipo,texto_ocr=texto,inicia_documento=inicio,confianca=.95,campos={"assinatura_detectada":assinatura})


def test_nao_junta_paginacao_ou_continuacao_de_cpfs_diferentes():
    a = _pagina("CONTRATO_TRABALHO", "CPF: 52998224725 Página 1/2")
    b = _pagina("CONTRATO_TRABALHO", "CPF: 11144477735 Página 2/2",False)
    assert len(agrupar.agrupar({1:a,2:b})) == 2
    b["texto_ocr"] = "CPF: 11144477735"
    assert len(agrupar.agrupar({1:a,2:b})) == 2


def test_assinatura_no_verso_completa_documento():
    a = _pagina("CONTRATO_TRABALHO", "CONTRATO DE TRABALHO Página 1/2",assinatura=False)
    b = _pagina("CONTRATO_TRABALHO", "Página 2/2",False,True)
    docs = agrupar.agrupar({1:a,2:b})
    assert len(docs)==1 and docs[0]["campos"]["assinatura_detectada"] is True
    assert not any(c["codigo"]=="DOCUMENTO_SEM_ASSINATURA" for c in docs[0]["nao_conformidades"])


@pytest.mark.parametrize("lado_a_lado", [False, True])
def test_recorta_dois_documentos_preservando_origem_e_texto(tmp_path,lado_a_lado):
    doc=fitz.open();p=doc.new_page(width=800,height=800)
    for x,y,titulo in [(30,40,"RECIBO DE PAGAMENTO"),(430,40,"FICHA DE EPI")] if lado_a_lado else [(30,40,"RECIBO DE PAGAMENTO"),(30,440,"FICHA DE EPI")]:
        p.insert_text((x,y),titulo)
        p.insert_text((x,y+25),"Nome: Maria CPF: 529.982.247-25")
        p.insert_text((x,y+50),"Competencia 04/2026")
        p.draw_rect(fitz.Rect(x,y-20,x+300,y+200))
    src=tmp_path/'l.pdf';doc.save(src);doc.close()
    dest=tmp_path/'out.zip'
    assert fatiar.fatiar(str(src),'l.pdf',str(dest))==2
    with zipfile.ZipFile(dest) as z:
        m=json.loads(z.read('manifest.json'))
        assert m['total_folhas']==1
        assert [p['pagina_origem'] for p in m['paginas']]==[1,1]
        assert [p['regiao'] for p in m['paginas']]==[1,2]
        assert 'FICHA DE EPI' not in m['paginas'][0]['texto_embutido']
        assert 'RECIBO DE PAGAMENTO' not in m['paginas'][1]['texto_embutido']
        for p in m['paginas']:
            assert all(0<=v<=1 for w in p['palavras'] for v in w[:4])


def test_nao_recorta_espaco_entre_corpo_e_assinatura(tmp_path):
    doc=fitz.open();p=doc.new_page(width=600,height=800)
    p.insert_text((40,50),'CONTRATO DE TRABALHO')
    p.insert_text((40,100),'Clausulas e obrigacoes do empregado')
    p.insert_text((40,650),'Assinatura do empregado: _______')
    src=tmp_path/'l.pdf';doc.save(src);doc.close()
    assert fatiar.fatiar(str(src),'l.pdf',str(tmp_path/'out.zip'))==1


def test_assinatura_ausente_na_frente_nao_resolve_verso_incerto():
    a = _pagina("CONTRATO_TRABALHO", "Página 1/2", assinatura=False)
    b = _pagina("CONTRATO_TRABALHO", "Página 2/2", False, None)
    doc = agrupar.agrupar({1:a, 2:b})[0]
    assert doc['campos']['assinatura_detectada'] is None
    assert 'ASSINATURA_NAO_VERIFICADA' in doc['motivos_revisao']


def test_limiar_do_lote_aplicado_no_checklist():
    p = _pagina('CNH', 'CNH')
    p['confianca'] = .8
    d = agrupar.agrupar({1:p},limiar=.9)[0]
    assert next(c['status'] for c in d['checklist'] if c['regra']=='Legibilidade da leitura')=='REVISAO_HUMANA'


def test_frente_verso_na_mesma_folha_recorta_e_reagrupa(tmp_path):
    from ia_core.providers import LocalProvider
    doc=fitz.open();p=doc.new_page(width=800,height=800)
    for x,titulo in [(30,'AVISO DE RECEBIMENTO'),(430,'DADOS DO RECEBIMENTO')]:
        p.insert_text((x,40),titulo)
        p.insert_text((x,70),'Codigo do objeto: AB123456789BR')
        p.insert_text((x,100),'Nome: Maria CPF: 529.982.247-25')
        p.draw_rect(fitz.Rect(x,20,x+300,300))
    src=tmp_path/'ar.pdf';doc.save(src);doc.close()
    dest=tmp_path/'out.zip'
    assert fatiar.fatiar(str(src),'ar.pdf',str(dest))==2
    with zipfile.ZipFile(dest) as z:
        ps=json.loads(z.read('manifest.json'))['paginas']
        prov=LocalProvider();analises={};anterior=None
        for p in ps:
            anterior=prov.analisar(z.read(f"pages/{p['numero']:04d}.png"),p['numero'],anterior,p['texto_embutido'],'MISTO')
            analises[p['numero']]=anterior
    docs=agrupar.agrupar(analises)
    assert len(docs)==1 and docs[0]['tipo']=='AVISO_RECEBIMENTO'
    assert docs[0]['paginas']==[1,2]


def test_raciocinio_conta_na_saida_sem_tarifa(monkeypatch):
    monkeypatch.delenv('IA_PRICES_BRL_PER_MILLION',raising=False)
    c=chamada('modelo',1000,600,100)
    assert c['saida']==400 and c['custo_estimado_brl'] is None
