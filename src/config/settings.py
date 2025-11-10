"""
Система конфигурации Allan.
Поддерживает загрузку из переменных окружения, YAML файлов и значений по умолчанию.
"""

import os
import yaml
from pathlib import Path
from typing import Optional, Dict, Any
from dataclasses import dataclass, field


@dataclass
class AllanConfig:
    """Основная конфигурация Allan."""

    # Пути проекта
    project_path: str = field(default_factory=lambda: os.getenv(
        'ALLAN_PROJECT_PATH',
        '/content/drive/MyDrive/ML_Projects/Allan_Model'
    ))
    local_cache_path: str = field(default_factory=lambda: os.getenv(
        'ALLAN_CACHE_PATH',
        '/content/allan_cache'
    ))

    # Настройки датасетов
    datasets_path: Optional[str] = None
    raw_datasets_path: Optional[str] = None
    processed_datasets_path: Optional[str] = None
    cached_datasets_path: Optional[str] = None

    # Настройки моделей
    models_path: Optional[str] = None
    checkpoints_path: Optional[str] = None

    # Лимиты ресурсов (в МБ)
    small_dataset_threshold: int = 100
    medium_dataset_threshold: int = 500
    large_dataset_threshold: int = 2000

    # Пороги производительности
    ram_warning_threshold: float = 0.75  # 75%
    ram_critical_threshold: float = 0.90  # 90%
    disk_warning_threshold: float = 0.80  # 80%
    disk_critical_threshold: float = 0.95  # 95%

    # Настройки окружения
    is_colab: bool = field(default_factory=lambda: 'COLAB_GPU' in os.environ)
    device: str = field(default_factory=lambda: os.getenv('ALLAN_DEVICE', 'auto'))

    # Hugging Face
    hf_cache_dir: Optional[str] = field(default_factory=lambda: os.getenv('HF_HOME'))
    hf_token: Optional[str] = field(default_factory=lambda: os.getenv('HF_TOKEN'))

    # Логирование
    log_level: str = field(default_factory=lambda: os.getenv('ALLAN_LOG_LEVEL', 'INFO'))

    def __post_init__(self):
        """Инициализация зависимых путей."""
        # Автоматически установить пути на основе project_path
        if self.datasets_path is None:
            self.datasets_path = str(Path(self.project_path) / "datasets")

        if self.raw_datasets_path is None:
            self.raw_datasets_path = str(Path(self.datasets_path) / "raw")

        if self.processed_datasets_path is None:
            self.processed_datasets_path = str(Path(self.datasets_path) / "processed")

        if self.cached_datasets_path is None:
            self.cached_datasets_path = str(Path(self.datasets_path) / "cached")

        if self.models_path is None:
            self.models_path = str(Path(self.project_path) / "models")

        if self.checkpoints_path is None:
            self.checkpoints_path = str(Path(self.models_path) / "checkpoints")

    def to_dict(self) -> Dict[str, Any]:
        """Конвертировать конфигурацию в словарь."""
        return {
            'project_path': self.project_path,
            'local_cache_path': self.local_cache_path,
            'datasets_path': self.datasets_path,
            'raw_datasets_path': self.raw_datasets_path,
            'processed_datasets_path': self.processed_datasets_path,
            'cached_datasets_path': self.cached_datasets_path,
            'models_path': self.models_path,
            'checkpoints_path': self.checkpoints_path,
            'small_dataset_threshold': self.small_dataset_threshold,
            'medium_dataset_threshold': self.medium_dataset_threshold,
            'large_dataset_threshold': self.large_dataset_threshold,
            'ram_warning_threshold': self.ram_warning_threshold,
            'ram_critical_threshold': self.ram_critical_threshold,
            'disk_warning_threshold': self.disk_warning_threshold,
            'disk_critical_threshold': self.disk_critical_threshold,
            'is_colab': self.is_colab,
            'device': self.device,
            'log_level': self.log_level,
        }

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'AllanConfig':
        """Создать конфигурацию из словаря."""
        return cls(**data)

    @classmethod
    def from_yaml(cls, yaml_path: str) -> 'AllanConfig':
        """Загрузить конфигурацию из YAML файла."""
        with open(yaml_path, 'r', encoding='utf-8') as f:
            data = yaml.safe_load(f)
        return cls.from_dict(data)

    def save_yaml(self, yaml_path: str):
        """Сохранить конфигурацию в YAML файл."""
        with open(yaml_path, 'w', encoding='utf-8') as f:
            yaml.dump(self.to_dict(), f, default_flow_style=False, allow_unicode=True)


# Глобальная конфигурация
_global_config: Optional[AllanConfig] = None


def get_config() -> AllanConfig:
    """
    Получить глобальную конфигурацию.
    Если конфигурация еще не загружена, создается с настройками по умолчанию.
    """
    global _global_config
    if _global_config is None:
        _global_config = AllanConfig()
    return _global_config


def load_config(config_path: Optional[str] = None, **kwargs) -> AllanConfig:
    """
    Загрузить конфигурацию из файла или создать с параметрами.

    Args:
        config_path: Путь к YAML файлу конфигурации
        **kwargs: Параметры конфигурации для переопределения

    Returns:
        AllanConfig: Загруженная конфигурация
    """
    global _global_config

    if config_path and Path(config_path).exists():
        _global_config = AllanConfig.from_yaml(config_path)
    else:
        _global_config = AllanConfig(**kwargs)

    return _global_config
