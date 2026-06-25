import asyncio
import logging

import voce
from voce import Config

# This import triggers the @plugin decorators to register the example plugins
import plugins

async def main():
    logging.basicConfig(level=logging.INFO)
    logger = logging.getLogger(__name__)
    
    config = Config(host="127.0.0.1", port=50051)
    app = voce.App(config)
    
    # We can either await app.start() then await app.stop(), or use app.serve()
    await app.serve()

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
