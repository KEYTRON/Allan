"""
Загрузчик датасетов для Allan.
Поддерживает загрузку из corus (русские корпуса) и Google Drive.
"""

import os
from pathlib import Path
from typing import Iterator, List, Optional, Dict, Any
from tqdm import tqdm


class CorusDataLoader:
    """
    Загрузчик русскоязычных корпусов с использованием библиотеки corus.
    """

    def __init__(self, cache_dir: str = "./data/corus_cache"):
        """
        Args:
            cache_dir: Директория для кеширования скачанных корпусов
        """
        self.cache_dir = Path(cache_dir)
        self.cache_dir.mkdir(parents=True, exist_ok=True)

    def load_taiga(self, subset: str = "proza") -> Iterator[str]:
        """
        Загрузить корпус Taiga.

        Args:
            subset: Подмножество ('proza', 'social', 'news', etc.)

        Yields:
            Тексты из корпуса
        """
        try:
            from corus import load_taiga_proza, load_taiga_social, load_taiga_fontanka
        except ImportError:
            raise ImportError("Установите corus: pip install corus")

        loader_map = {
            'proza': load_taiga_proza,
            'social': load_taiga_social,
            'fontanka': load_taiga_fontanka,
        }

        if subset not in loader_map:
            raise ValueError(f"Неизвестный subset: {subset}. Доступны: {list(loader_map.keys())}")

        print(f"Загрузка Taiga {subset}...")
        records = loader_map[subset]()

        for record in records:
            if hasattr(record, 'text'):
                yield record.text

    def load_lenta(self) -> Iterator[str]:
        """
        Загрузить корпус Lenta.ru.

        Yields:
            Статьи из Lenta.ru
        """
        try:
            from corus import load_lenta
        except ImportError:
            raise ImportError("Установите corus: pip install corus")

        print("Загрузка Lenta.ru...")
        records = load_lenta()

        for record in records:
            yield record.text

    def load_ria(self) -> Iterator[str]:
        """
        Загрузить корпус РИА новости.

        Yields:
            Статьи из РИА
        """
        try:
            from corus import load_ria
        except ImportError:
            raise ImportError("Установите corus: pip install corus")

        print("Загрузка РИА...")
        records = load_ria()

        for record in records:
            yield record.text

    def load_wikiner(self) -> Iterator[str]:
        """
        Загрузить WikiNER русский.

        Yields:
            Тексты из WikiNER
        """
        try:
            from corus import load_wikiner
        except ImportError:
            raise ImportError("Установите corus: pip install corus")

        print("Загрузка WikiNER...")
        records = load_wikiner('ru')

        for record in records:
            # WikiNER содержит токены, объединяем их
            if hasattr(record, 'tokens'):
                text = ' '.join(token.text for token in record.tokens)
                yield text

    def load_all_available(
        self,
        max_texts: Optional[int] = None
    ) -> Iterator[str]:
        """
        Загрузить все доступные корпуса.

        Args:
            max_texts: Максимальное количество текстов (None = все)

        Yields:
            Тексты из всех корпусов
        """
        count = 0

        # Список всех доступных загрузчиков
        loaders = [
            ('Lenta.ru', self.load_lenta),
            ('RIA', self.load_ria),
            ('Taiga Proza', lambda: self.load_taiga('proza')),
            ('Taiga Social', lambda: self.load_taiga('social')),
        ]

        for name, loader in loaders:
            print(f"\n=== Загрузка {name} ===")
            try:
                for text in loader():
                    yield text
                    count += 1

                    if max_texts and count >= max_texts:
                        print(f"\nДостигнут лимит {max_texts} текстов")
                        return

            except Exception as e:
                print(f"Ошибка при загрузке {name}: {e}")
                continue

        print(f"\nВсего загружено текстов: {count}")


class GoogleDriveDataLoader:
    """
    Загрузчик датасетов с Google Drive.
    """

    def __init__(self, drive_path: str = "/content/drive/MyDrive"):
        """
        Args:
            drive_path: Путь к смонтированному Google Drive
        """
        self.drive_path = Path(drive_path)

    def load_jsonl(self, file_path: str) -> Iterator[Dict[str, Any]]:
        """
        Загрузить JSONL файл с Google Drive.

        Args:
            file_path: Путь к файлу относительно drive_path

        Yields:
            Строки из JSONL файла
        """
        import json

        full_path = self.drive_path / file_path

        if not full_path.exists():
            raise FileNotFoundError(f"Файл не найден: {full_path}")

        print(f"Загрузка {file_path}...")

        with open(full_path, 'r', encoding='utf-8') as f:
            for line in f:
                if line.strip():
                    yield json.loads(line)

    def load_text_file(self, file_path: str, chunk_size: int = 1000) -> Iterator[str]:
        """
        Загрузить текстовый файл построчно.

        Args:
            file_path: Путь к файлу
            chunk_size: Размер чанка для отображения прогресса

        Yields:
            Строки из файла
        """
        full_path = self.drive_path / file_path

        if not full_path.exists():
            raise FileNotFoundError(f"Файл не найден: {full_path}")

        print(f"Загрузка {file_path}...")

        with open(full_path, 'r', encoding='utf-8') as f:
            for i, line in enumerate(f):
                yield line.strip()

                if (i + 1) % chunk_size == 0:
                    print(f"Загружено строк: {i + 1}")

    def load_directory(
        self,
        dir_path: str,
        pattern: str = "*.txt",
        recursive: bool = True
    ) -> Iterator[str]:
        """
        Загрузить все файлы из директории.

        Args:
            dir_path: Путь к директории
            pattern: Паттерн файлов (glob)
            recursive: Рекурсивный поиск

        Yields:
            Содержимое файлов
        """
        full_path = self.drive_path / dir_path

        if not full_path.exists():
            raise FileNotFoundError(f"Директория не найдена: {full_path}")

        # Поиск файлов
        if recursive:
            files = list(full_path.rglob(pattern))
        else:
            files = list(full_path.glob(pattern))

        print(f"Найдено файлов: {len(files)}")

        for file_path in tqdm(files, desc="Загрузка файлов"):
            try:
                with open(file_path, 'r', encoding='utf-8') as f:
                    yield f.read()
            except Exception as e:
                print(f"Ошибка при чтении {file_path}: {e}")
                continue


class DatasetPreprocessor:
    """
    Препроцессор для подготовки текстов к обучению.
    """

    def __init__(self, tokenizer):
        """
        Args:
            tokenizer: Токенизатор Allan
        """
        self.tokenizer = tokenizer

    def clean_text(self, text: str) -> str:
        """
        Очистка текста.

        Args:
            text: Входной текст

        Returns:
            Очищенный текст
        """
        # Удаляем лишние пробелы
        text = ' '.join(text.split())

        # Удаляем очень короткие тексты
        if len(text) < 10:
            return ""

        return text

    def create_training_examples(
        self,
        texts: Iterator[str],
        max_length: int = 512,
        stride: int = 256,
    ) -> Iterator[List[int]]:
        """
        Создать примеры для обучения из текстов.

        Args:
            texts: Итератор текстов
            max_length: Максимальная длина последовательности
            stride: Шаг скользящего окна

        Yields:
            Токенизированные последовательности
        """
        buffer = []

        for text in texts:
            # Очищаем текст
            text = self.clean_text(text)
            if not text:
                continue

            # Токенизируем
            tokens = self.tokenizer.encode(text, add_bos=False, add_eos=False)

            # Добавляем в буфер
            buffer.extend(tokens)

            # Создаем примеры скользящим окном
            while len(buffer) >= max_length:
                example = buffer[:max_length]
                yield example

                # Сдвигаем окно
                buffer = buffer[stride:]

        # Обрабатываем остаток буфера
        if len(buffer) > 0:
            # Паддинг до max_length
            while len(buffer) < max_length:
                buffer.append(self.tokenizer.pad_token_id)

            yield buffer[:max_length]


def prepare_training_data(
    data_source: str,
    tokenizer,
    save_path: str,
    max_samples: Optional[int] = None,
    max_length: int = 512,
):
    """
    Подготовить данные для обучения.

    Args:
        data_source: Источник данных ('corus', путь к файлу, или директории)
        tokenizer: Токенизатор Allan
        save_path: Путь для сохранения подготовленных данных
        max_samples: Максимальное количество примеров
        max_length: Максимальная длина последовательности
    """
    import torch
    import numpy as np

    # Загружаем тексты
    if data_source == 'corus':
        loader = CorusDataLoader()
        texts = loader.load_all_available(max_texts=max_samples)
    elif os.path.isfile(data_source):
        # Один файл
        drive_loader = GoogleDriveDataLoader()
        if data_source.endswith('.jsonl'):
            records = drive_loader.load_jsonl(data_source)
            texts = (r.get('text', '') for r in records)
        else:
            texts = drive_loader.load_text_file(data_source)
    elif os.path.isdir(data_source):
        # Директория с файлами
        drive_loader = GoogleDriveDataLoader()
        texts = drive_loader.load_directory(data_source)
    else:
        raise ValueError(f"Неизвестный источник данных: {data_source}")

    # Препроцессор
    preprocessor = DatasetPreprocessor(tokenizer)

    # Создаем примеры
    print("Создание обучающих примеров...")
    examples = []

    for example in tqdm(
        preprocessor.create_training_examples(texts, max_length=max_length),
        desc="Обработка"
    ):
        examples.append(example)

        if max_samples and len(examples) >= max_samples:
            break

    # Конвертируем в тензоры
    print(f"Создано примеров: {len(examples)}")
    data_tensor = torch.tensor(examples, dtype=torch.long)

    # Сохраняем
    os.makedirs(os.path.dirname(save_path), exist_ok=True)
    torch.save(data_tensor, save_path)

    print(f"Данные сохранены в {save_path}")
    print(f"Размер: {data_tensor.shape}")

    return data_tensor
