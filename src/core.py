"""
Allan Core - Основной модуль фреймворка Allan.
Предоставляет базовую функциональность для работы с моделями и датасетами.
"""

import logging
from typing import Optional, Dict, Any, List
from pathlib import Path

try:
    import torch
    TORCH_AVAILABLE = True
except ImportError:
    TORCH_AVAILABLE = False

try:
    from transformers import AutoTokenizer, AutoModelForCausalLM, AutoConfig
    TRANSFORMERS_AVAILABLE = True
except ImportError:
    TRANSFORMERS_AVAILABLE = False

from .config import get_config, AllanConfig

logger = logging.getLogger(__name__)


class Allan:
    """
    Основной класс фреймворка Allan.

    Предоставляет интерфейс для:
    - Загрузки и работы с языковыми моделями
    - Управления датасетами
    - Обучения и fine-tuning
    - Inference и генерации текста
    """

    def __init__(
        self,
        model_name: Optional[str] = None,
        config: Optional[AllanConfig] = None,
        device: Optional[str] = None,
        **kwargs
    ):
        """
        Инициализация Allan.

        Args:
            model_name: Название модели из Hugging Face или локальный путь
            config: Конфигурация Allan (если None, используется глобальная)
            device: Устройство для вычислений ('cpu', 'cuda', 'mps', 'auto')
            **kwargs: Дополнительные параметры
        """
        self.config = config or get_config()
        self.model_name = model_name
        self.device = device or self._auto_detect_device()

        self.model = None
        self.tokenizer = None
        self.model_config = None

        logger.info(f"Allan инициализирован. Устройство: {self.device}")

        if model_name:
            logger.info(f"Запланирована загрузка модели: {model_name}")

    def _auto_detect_device(self) -> str:
        """Автоматическое определение доступного устройства."""
        if not TORCH_AVAILABLE:
            return 'cpu'

        if self.config.device != 'auto':
            return self.config.device

        if torch.cuda.is_available():
            return 'cuda'
        elif hasattr(torch.backends, 'mps') and torch.backends.mps.is_available():
            return 'mps'
        else:
            return 'cpu'

    def load_model(
        self,
        model_name: Optional[str] = None,
        load_in_8bit: bool = False,
        load_in_4bit: bool = False,
        **kwargs
    ) -> bool:
        """
        Загрузить модель из Hugging Face или локального пути.

        Args:
            model_name: Название модели (если None, используется self.model_name)
            load_in_8bit: Загрузить модель в 8-bit режиме (требует bitsandbytes)
            load_in_4bit: Загрузить модель в 4-bit режиме (требует bitsandbytes)
            **kwargs: Дополнительные параметры для AutoModel

        Returns:
            bool: True если модель загружена успешно
        """
        if not TRANSFORMERS_AVAILABLE:
            logger.error("transformers не установлен. Установите: pip install transformers")
            return False

        model_name = model_name or self.model_name
        if not model_name:
            logger.error("Не указано название модели")
            return False

        try:
            logger.info(f"Загрузка модели: {model_name}")

            # Загрузка конфигурации
            self.model_config = AutoConfig.from_pretrained(
                model_name,
                cache_dir=self.config.hf_cache_dir,
                token=self.config.hf_token
            )

            # Загрузка токенизатора
            self.tokenizer = AutoTokenizer.from_pretrained(
                model_name,
                cache_dir=self.config.hf_cache_dir,
                token=self.config.hf_token
            )

            # Параметры загрузки модели
            model_kwargs = {
                'cache_dir': self.config.hf_cache_dir,
                'token': self.config.hf_token,
                **kwargs
            }

            if load_in_8bit or load_in_4bit:
                model_kwargs['load_in_8bit'] = load_in_8bit
                model_kwargs['load_in_4bit'] = load_in_4bit
                model_kwargs['device_map'] = 'auto'
            else:
                model_kwargs['device_map'] = self.device

            # Загрузка модели
            self.model = AutoModelForCausalLM.from_pretrained(
                model_name,
                config=self.model_config,
                **model_kwargs
            )

            logger.info(f"Модель {model_name} успешно загружена на {self.device}")
            return True

        except Exception as e:
            logger.error(f"Ошибка при загрузке модели: {e}")
            return False

    def generate(
        self,
        prompt: str,
        max_length: int = 100,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 50,
        **kwargs
    ) -> Optional[str]:
        """
        Генерация текста на основе промпта.

        Args:
            prompt: Входной текст
            max_length: Максимальная длина генерации
            temperature: Температура сэмплирования
            top_p: Nucleus sampling параметр
            top_k: Top-k sampling параметр
            **kwargs: Дополнительные параметры генерации

        Returns:
            str: Сгенерированный текст или None при ошибке
        """
        if not self.model or not self.tokenizer:
            logger.error("Модель не загружена. Вызовите load_model() сначала")
            return None

        try:
            inputs = self.tokenizer(prompt, return_tensors="pt").to(self.device)

            outputs = self.model.generate(
                **inputs,
                max_length=max_length,
                temperature=temperature,
                top_p=top_p,
                top_k=top_k,
                do_sample=True,
                **kwargs
            )

            generated_text = self.tokenizer.decode(outputs[0], skip_special_tokens=True)
            return generated_text

        except Exception as e:
            logger.error(f"Ошибка при генерации: {e}")
            return None

    def get_info(self) -> Dict[str, Any]:
        """
        Получить информацию о текущем состоянии Allan.

        Returns:
            dict: Информация о модели, конфигурации и системе
        """
        info = {
            'model_loaded': self.model is not None,
            'model_name': self.model_name,
            'device': self.device,
            'config': self.config.to_dict(),
            'torch_available': TORCH_AVAILABLE,
            'transformers_available': TRANSFORMERS_AVAILABLE,
        }

        if TORCH_AVAILABLE:
            info['cuda_available'] = torch.cuda.is_available()
            if torch.cuda.is_available():
                info['cuda_device_count'] = torch.cuda.device_count()
                info['cuda_device_name'] = torch.cuda.get_device_name(0)

        return info

    def run(self):
        """Запуск Allan в интерактивном режиме (для совместимости)."""
        logger.info("Allan готов к работе")
        info = self.get_info()

        print("=" * 50)
        print("Allan - Framework для русскоязычных LLM")
        print("=" * 50)
        print(f"Устройство: {info['device']}")
        print(f"Модель загружена: {info['model_loaded']}")
        if info['model_loaded']:
            print(f"Название модели: {info['model_name']}")
        print(f"PyTorch доступен: {info['torch_available']}")
        print(f"Transformers доступен: {info['transformers_available']}")
        if 'cuda_available' in info and info['cuda_available']:
            print(f"CUDA устройство: {info.get('cuda_device_name', 'N/A')}")
        print("=" * 50)
