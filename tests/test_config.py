"""Тесты для модуля конфигурации."""

import os
import pytest
from pathlib import Path
import tempfile
import yaml

# Добавляем src в путь для импорта
import sys
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from config.settings import AllanConfig, get_config, load_config


class TestAllanConfig:
    """Тесты для класса AllanConfig."""

    def test_config_creation_with_defaults(self):
        """Тест создания конфигурации с настройками по умолчанию."""
        config = AllanConfig()

        assert config.project_path is not None
        assert config.local_cache_path is not None
        assert config.small_dataset_threshold == 100
        assert config.medium_dataset_threshold == 500
        assert config.large_dataset_threshold == 2000

    def test_config_paths_auto_generation(self):
        """Тест автоматической генерации путей."""
        config = AllanConfig(project_path="/test/path")

        assert config.datasets_path == "/test/path/datasets"
        assert config.raw_datasets_path == "/test/path/datasets/raw"
        assert config.processed_datasets_path == "/test/path/datasets/processed"
        assert config.cached_datasets_path == "/test/path/datasets/cached"
        assert config.models_path == "/test/path/models"
        assert config.checkpoints_path == "/test/path/models/checkpoints"

    def test_config_from_dict(self):
        """Тест создания конфигурации из словаря."""
        data = {
            'project_path': '/custom/path',
            'small_dataset_threshold': 200,
            'ram_warning_threshold': 0.80
        }
        config = AllanConfig.from_dict(data)

        assert config.project_path == '/custom/path'
        assert config.small_dataset_threshold == 200
        assert config.ram_warning_threshold == 0.80

    def test_config_to_dict(self):
        """Тест конвертации конфигурации в словарь."""
        config = AllanConfig(project_path='/test')
        data = config.to_dict()

        assert isinstance(data, dict)
        assert 'project_path' in data
        assert 'datasets_path' in data
        assert data['project_path'] == '/test'

    def test_config_save_and_load_yaml(self):
        """Тест сохранения и загрузки конфигурации из YAML."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            temp_path = f.name

        try:
            # Создаем и сохраняем конфигурацию
            config1 = AllanConfig(
                project_path='/test/path',
                small_dataset_threshold=150
            )
            config1.save_yaml(temp_path)

            # Загружаем конфигурацию
            config2 = AllanConfig.from_yaml(temp_path)

            assert config2.project_path == '/test/path'
            assert config2.small_dataset_threshold == 150

        finally:
            if os.path.exists(temp_path):
                os.remove(temp_path)

    def test_device_detection(self):
        """Тест определения устройства."""
        config = AllanConfig()
        assert config.device in ['auto', 'cpu', 'cuda', 'mps']

    def test_thresholds_are_reasonable(self):
        """Тест что пороговые значения разумные."""
        config = AllanConfig()

        assert 0 < config.ram_warning_threshold < 1
        assert 0 < config.ram_critical_threshold < 1
        assert config.ram_warning_threshold < config.ram_critical_threshold
        assert 0 < config.disk_warning_threshold < 1
        assert 0 < config.disk_critical_threshold < 1


class TestGlobalConfig:
    """Тесты для глобальной конфигурации."""

    def test_get_config_returns_config(self):
        """Тест что get_config возвращает конфигурацию."""
        config = get_config()
        assert isinstance(config, AllanConfig)

    def test_load_config_with_params(self):
        """Тест загрузки конфигурации с параметрами."""
        config = load_config(project_path='/custom/path')
        assert config.project_path == '/custom/path'


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
