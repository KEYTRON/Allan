# src/main.py

import logging
from src.core import Allan

def setup_logging():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(message)s"
    )

def main():
    setup_logging()
    logging.info("🚀 Allan запускается...")

    # Инициализация ядра
    bot = Allan()
    bot.run()

if __name__ == "__main__":
    main()