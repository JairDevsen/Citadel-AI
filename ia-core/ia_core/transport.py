"""Clientes HTTP reaproveitados pelos workers: conexão persistente, limites e timeouts."""
import atexit

import httpx

DOWNLOAD = httpx.Client(timeout=120, limits=httpx.Limits(max_connections=16, max_keepalive_connections=8))
CALLBACK = httpx.Client(timeout=30, limits=httpx.Limits(max_connections=16, max_keepalive_connections=8))


@atexit.register
def fechar():
    DOWNLOAD.close()
    CALLBACK.close()
