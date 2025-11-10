"""
Allan - Фреймворк для обучения и использования русскоязычных языковых моделей.

Основные компоненты:
- core: Основная функциональность для работы с моделями
- config: Система конфигурации
- datasets: Управление датасетами
- optimization: Оптимизация производительности
- colab: Интеграция с Google Colab
- nlp: NLP утилиты для русского языка
"""

__version__ = "0.2.0"
__author__ = "KEYTRON"

from .core import Allan
from .config import AllanConfig, get_config, load_config

__all__ = [
    "Allan",
    "AllanConfig",
    "get_config",
    "load_config",
    "__version__",
]
