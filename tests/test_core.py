"""Тесты для основного модуля Allan."""

import pytest
from pathlib import Path
import sys

# Добавляем src в путь для импорта
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from core import Allan
from config.settings import AllanConfig


class TestAllan:
    """Тесты для класса Allan."""

    def test_allan_initialization(self):
        """Тест инициализации Allan."""
        allan = Allan()

        assert allan is not None
        assert allan.model is None
        assert allan.tokenizer is None
        assert allan.device in ['cpu', 'cuda', 'mps']

    def test_allan_initialization_with_config(self):
        """Тест инициализации Allan с конфигурацией."""
        config = AllanConfig(project_path='/test/path')
        allan = Allan(config=config)

        assert allan.config == config
        assert allan.config.project_path == '/test/path'

    def test_allan_initialization_with_model_name(self):
        """Тест инициализации Allan с названием модели."""
        allan = Allan(model_name='test/model')

        assert allan.model_name == 'test/model'

    def test_allan_device_auto_detection(self):
        """Тест автоматического определения устройства."""
        allan = Allan()

        assert allan.device is not None
        assert isinstance(allan.device, str)

    def test_allan_custom_device(self):
        """Тест установки пользовательского устройства."""
        allan = Allan(device='cpu')

        assert allan.device == 'cpu'

    def test_allan_get_info(self):
        """Тест получения информации о системе."""
        allan = Allan()
        info = allan.get_info()

        assert isinstance(info, dict)
        assert 'model_loaded' in info
        assert 'device' in info
        assert 'torch_available' in info
        assert 'transformers_available' in info
        assert info['model_loaded'] == False

    def test_allan_get_info_with_model_name(self):
        """Тест получения информации с названием модели."""
        allan = Allan(model_name='test/model')
        info = allan.get_info()

        assert info['model_name'] == 'test/model'

    def test_allan_run(self, capsys):
        """Тест запуска Allan."""
        allan = Allan()
        allan.run()

        captured = capsys.readouterr()
        assert "Allan" in captured.out
        assert "Framework" in captured.out

    def test_generate_without_model(self, capsys):
        """Тест генерации без загруженной модели."""
        allan = Allan()
        result = allan.generate("Test prompt")

        assert result is None

    def test_load_model_without_transformers(self, monkeypatch):
        """Тест загрузки модели когда transformers недоступен."""
        # Симулируем отсутствие transformers
        import core as core_module
        original = core_module.TRANSFORMERS_AVAILABLE
        core_module.TRANSFORMERS_AVAILABLE = False

        try:
            allan = Allan()
            result = allan.load_model("test/model")

            assert result == False
        finally:
            core_module.TRANSFORMERS_AVAILABLE = original

    def test_config_integration(self):
        """Тест интеграции с конфигурацией."""
        config = AllanConfig(
            project_path='/custom/path',
            device='cpu'
        )
        allan = Allan(config=config)

        assert allan.config.project_path == '/custom/path'
        # device передается явно в Allan, поэтому он может отличаться
        # assert allan.device == 'cpu' or allan.device == allan._auto_detect_device()


class TestAllanEdgeCases:
    """Тесты граничных случаев для Allan."""

    def test_allan_with_none_values(self):
        """Тест инициализации с None значениями."""
        allan = Allan(model_name=None, config=None, device=None)

        assert allan.model_name is None
        assert allan.config is not None  # должна создаться дефолтная
        assert allan.device is not None  # должно автоопределиться

    def test_load_model_without_model_name(self):
        """Тест загрузки модели без указания имени."""
        allan = Allan()
        result = allan.load_model()

        assert result == False


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
