"""Regressões de custo/latência com chamadas simuladas: nenhuma chave ou documento real."""
import io
import time

import pytest
from PIL import Image

from ia_core import catalogo, main
from ia_core.impressao import impressao_motor
from ia_core.providers import LLMProvider, LocalProvider, Pedido, _Uso, compactar_texto

TEXTO = ('CONTRATO DE TRABALHO\nEMPREGADOR EMPREGADO CLAUSULA PRIMEIRA PERIODO DE EXPERIENCIA\n'
         'Nome: MARIA DA SILVA CPF: 529.982.247-25\nAssinatura do empregado: ______')


def png():
    out=io.BytesIO(); Image.new('RGB',(2400,3000),'white').save(out,'PNG');return out.getvalue()


class Simulado(LLMProvider):
    model, fallback, lite = 'principal', 'reserva', 'texto'

    def __init__(self, respostas):
        self.respostas=list(respostas);self.chamadas=[]

    def _chamar(self, model, imagem, prompt, mime):
        self.chamadas.append((model, prompt, Image.open(io.BytesIO(imagem)).size))
        return self.respostas.pop(0), _Uso(100)

    def _chamar_texto(self, model, texto, prompt):
        self.chamadas.append((model, prompt, texto))
        return self.respostas.pop(0), _Uso(50)


def pedido(camada=3, nativo=True):
    local=LocalProvider().analisar_texto(TEXTO,None,nativo)
    return Pedido(camada,png(),TEXTO,nativo,local,'transcrever tudo','NÃO o transcreva',tipo_local=local['tipo'])


def resposta(**extras):
    return {'tipo':'CONTRATO_TRABALHO','inicia_documento':True,'confianca':.95,'tem_assinatura':True,**extras}


def test_visao_reusa_texto_nativo_e_preserva_manuscrito():
    p=Simulado([resposta(texto_complementar='Assinado por Maria em 08/11/2026')])
    pg=p.executar(pedido())
    assert len(p.chamadas)==1 and 'TEXTO NATIVO' in p.chamadas[0][1]
    assert TEXTO in pg['texto_ocr'] and '08/11/2026' in pg['texto_ocr']
    assert pg['campos']['assinatura_detectada'] is True
    assert pg['desempenho']['texto_nativo_reusado'] is True
    assert max(p.chamadas[0][2])>=1800


def test_ocr_previo_nao_recebe_tratamento_de_texto_nativo():
    p=Simulado([resposta(texto_ocr='Conteúdo realmente lido na imagem')])
    pg=p.executar(pedido(nativo=False))
    assert 'TEXTO NATIVO' not in p.chamadas[0][1]
    assert pg['texto_ocr']=='Conteúdo realmente lido na imagem'
    assert not pg['desempenho']['texto_nativo_reusado']
    a=LocalProvider().analisar(b'',1,None,TEXTO,'MISTO',texto_nativo=False)
    assert a['camada'].startswith('Camada 1: OCR')


def test_reserva_pior_nao_substitui_leitura_mas_consumo_conta():
    p=Simulado([resposta(confianca=.55),resposta(confianca=.3,pagina_ilegivel=True)])
    pg=p.executar(pedido())
    assert pg['confianca']==.55 and pg['tokens']==200
    assert len(pg['desempenho']['etapas'])==2
    assert max(p.chamadas[1][2])>=2200


def test_reserva_nao_descarta_documento_lido_como_folha_branca():
    p=Simulado([resposta(confianca=.55,pagina_ilegivel=True),
                resposta(confianca=.99,pagina_em_branco=True)])
    pg=p.executar(pedido())
    assert not pg['pagina_em_branco'] and pg['tokens']==200
    assert TEXTO in pg['texto_ocr']


def test_schema_invalido_escala_e_contabiliza_resposta_descartada():
    p=Simulado([resposta(confianca=4),resposta()])
    pg=p.executar(pedido(camada=2))
    assert [c[0] for c in p.chamadas]==['texto','principal']
    assert pg['tokens']==150 and pg['desempenho']['etapas'][0]['status']=='erro'


def test_regex_cache_acompanha_pistas_novas_sem_congelar_catalogo():
    codigo='TESTE_PERFORMANCE'
    try:
        catalogo.TIPOS[codigo]={'nome':'Teste','pistas':['MARCADOR PRIMEIRO'],'assina':False}
        assert catalogo.pontuar('MARCADOR PRIMEIRO')[codigo]>0
        catalogo.TIPOS[codigo]['pistas']=['MARCADOR SEGUNDO']
        assert codigo not in catalogo.pontuar('MARCADOR PRIMEIRO')
        assert catalogo.pontuar('MARCADOR SEGUNDO')[codigo]>0
    finally:
        catalogo.TIPOS.pop(codigo,None)


def test_camada_local_nao_constroi_prompt_nem_repete_leitura(monkeypatch):
    import ia_core.providers as pv
    calls=[]
    texto='RECIBO DE PAGAMENTO SALARIO BASE LIQUIDO A RECEBER INSS CPF: 529.982.247-25 Competencia: 04/2026 Nome: Maria da Silva '*2
    def ler(*args):
        calls.append(1);return None,texto,True
    monkeypatch.setattr(pv,'leitura_local',ler)
    p=Simulado([])
    monkeypatch.setattr(p,'_prompts',lambda *args:pytest.fail('prompt na camada local'))
    pg=p.preparar(b'',1,None,texto,'MISTO')
    assert pg['tokens']==0 and len(calls)==1


def test_texto_longo_nao_e_truncado_silenciosamente(monkeypatch):
    import ia_core.providers as pv
    monkeypatch.setattr(pv,'MAX_TEXTO',1000)
    texto=('DEMONSTRATIVO DE VALORES SOBRE PRESTACAO DE SERVICOS\n'*100)+'CPF: 529.982.247-25'
    p=Simulado([]);r=p.preparar(b'',1,None,texto,'MISTO')
    assert isinstance(r,Pedido) and r.camada==3 and r.texto.endswith('529.982.247-25')
    assert compactar_texto('Valor  100,00\n\nCPF:\t52998224725')=='Valor 100,00\nCPF: 52998224725'


def test_prefixo_estavel_para_cache_de_prompt():
    p=Simulado([])
    a,_=p._prompts({'tipo':'CNH'},'MISTO');b,_=p._prompts({'tipo':'ASO'},'RH')
    assert a.split('Contexto variável')[0]==b.split('Contexto variável')[0]
    assert p._schema() is Simulado([])._schema()


def test_preco_nao_invalida_cache_de_leitura_mas_roteamento_sim(monkeypatch):
    p=Simulado([])
    a=impressao_motor(p)
    monkeypatch.setenv('IA_PRICES_BRL_PER_MILLION','{"principal":{"input":99,"output":99}}')
    assert impressao_motor(p)==a
    monkeypatch.setenv('IA_TEXT_MAX_CHARS','9876')
    assert impressao_motor(p)!=a


def test_deduplicacao_compartilha_chamada_em_andamento_sem_dobrar_tokens(monkeypatch):
    eventos=[]
    class P:
        nome='simulado'
        def __init__(self):self.n=0
        def preparar(self,img,n,ant,texto,tipo_lote):
            return Pedido(3,img,'',False,None,'','',tipo_local='CNH')
        def executar(self,r):
            self.n+=1;time.sleep(.03)
            return {'tipo':'CNH','confianca':.95,'inicia_documento':True,'texto_ocr':'CNH',
                    'campos':{},'tokens':100,'uso':{'custo_estimado_brl':.01,'chamadas':[]},'camada':'Camada 3: Visão'}
    p=P();monkeypatch.setattr(main,'PROVIDER',p)
    monkeypatch.setattr(main,'_baixar',lambda url:b'capa' if url=='1' else b'mesma imagem')
    monkeypatch.setattr(main.callback,'enviar',lambda cb,corpo,chave:eventos.append(corpo))
    job=main.JobIn(batch_id='a',versao=1,callback_url='http://test',paginas=[main.PaginaIn(numero=n,image_url=str(n)) for n in range(1,6)])
    main._processar(job)
    paginas=[e['analise'] for e in eventos if e['evento']=='pagina']
    assert p.n==2 and sum(a['tokens'] for a in paginas)==200
    assert [e['numero'] for e in eventos if e['evento']=='pagina']==[1,2,3,4,5]
    assert paginas[2]['uso']['custo_estimado_brl']==0 and paginas[1]['tokens']==100
    assert paginas[2]['camada'].startswith('Camada 0')
    assert 'modelo_ms' not in paginas[2]['desempenho']
    eventos.clear(); main._processar(job)
    assert p.n==4, 'reaproveitamento não sai do lote/executação'
