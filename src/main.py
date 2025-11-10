"""
Allan main - Точка входа в приложение.
"""

import logging
import argparse
import sys
from pathlib import Path

from .core import Allan
from .config import load_config, AllanConfig


def setup_logging(level: str = "INFO"):
    """Настройка логирования."""
    numeric_level = getattr(logging, level.upper(), None)
    if not isinstance(numeric_level, int):
        raise ValueError(f'Неверный уровень логирования: {level}')

    logging.basicConfig(
        level=numeric_level,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S"
    )


def parse_args():
    """Парсинг аргументов командной строки."""
    parser = argparse.ArgumentParser(
        description="Allan - Framework для русскоязычных LLM"
    )

    parser.add_argument(
        "--config",
        type=str,
        help="Путь к файлу конфигурации YAML"
    )

    parser.add_argument(
        "--model",
        type=str,
        help="Название модели из HuggingFace или локальный путь"
    )

    parser.add_argument(
        "--device",
        type=str,
        choices=["auto", "cpu", "cuda", "mps"],
        default="auto",
        help="Устройство для вычислений"
    )

    parser.add_argument(
        "--log-level",
        type=str,
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        default="INFO",
        help="Уровень логирования"
    )

    parser.add_argument(
        "--interactive",
        action="store_true",
        help="Запустить в интерактивном режиме"
    )

    parser.add_argument(
        "--generate",
        type=str,
        help="Генерировать текст на основе промпта"
    )

    parser.add_argument(
        "--max-length",
        type=int,
        default=100,
        help="Максимальная длина генерации"
    )

    return parser.parse_args()


def main():
    """Основная функция."""
    args = parse_args()

    # Настройка логирования
    setup_logging(args.log_level)
    logger = logging.getLogger(__name__)

    logger.info("Allan запускается...")

    try:
        # Загрузка конфигурации
        config = None
        if args.config and Path(args.config).exists():
            logger.info(f"Загрузка конфигурации из {args.config}")
            config = load_config(args.config)
        else:
            config = load_config(device=args.device)

        # Инициализация Allan
        allan = Allan(
            model_name=args.model,
            config=config,
            device=args.device
        )

        # Режим генерации
        if args.generate:
            if not args.model:
                logger.error("Для генерации необходимо указать модель через --model")
                sys.exit(1)

            logger.info(f"Загрузка модели {args.model}...")
            if not allan.load_model():
                logger.error("Не удалось загрузить модель")
                sys.exit(1)

            logger.info(f"Генерация текста для промпта: {args.generate}")
            result = allan.generate(
                prompt=args.generate,
                max_length=args.max_length
            )

            if result:
                print("\n" + "=" * 50)
                print("Результат генерации:")
                print("=" * 50)
                print(result)
                print("=" * 50 + "\n")
            else:
                logger.error("Генерация не удалась")
                sys.exit(1)

        # Интерактивный режим или показ информации
        elif args.interactive:
            logger.info("Интерактивный режим пока не реализован")
            allan.run()
        else:
            # Просто показать информацию
            allan.run()

        logger.info("Allan завершил работу")

    except KeyboardInterrupt:
        logger.info("\nПрерывание пользователем")
        sys.exit(0)
    except Exception as e:
        logger.error(f"Критическая ошибка: {e}", exc_info=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
