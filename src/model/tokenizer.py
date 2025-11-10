"""
Токенизатор Allan для русского языка.
Использует SentencePiece для работы с русским текстом.
"""

import os
import json
from typing import List, Optional, Union
from pathlib import Path


class AllanTokenizer:
    """
    Токенизатор для Allan, оптимизированный для русского языка.
    Использует SentencePiece для создания словаря.
    """

    def __init__(
        self,
        vocab_file: Optional[str] = None,
        vocab_size: int = 50000,
        pad_token: str = "<PAD>",
        unk_token: str = "<UNK>",
        bos_token: str = "<BOS>",
        eos_token: str = "<EOS>",
    ):
        """
        Инициализация токенизатора.

        Args:
            vocab_file: Путь к файлу модели SentencePiece
            vocab_size: Размер словаря
            pad_token: Токен паддинга
            unk_token: Токен для неизвестных слов
            bos_token: Токен начала последовательности
            eos_token: Токен конца последовательности
        """
        self.vocab_size = vocab_size
        self.pad_token = pad_token
        self.unk_token = unk_token
        self.bos_token = bos_token
        self.eos_token = eos_token

        self.sp_model = None
        self.vocab = {}
        self.reverse_vocab = {}

        if vocab_file and os.path.exists(vocab_file):
            self.load(vocab_file)

    def train(
        self,
        texts: Union[List[str], str],
        model_prefix: str = "allan_tokenizer",
        vocab_size: Optional[int] = None,
    ):
        """
        Обучить токенизатор на текстах.

        Args:
            texts: Список текстов или путь к файлу с текстами
            model_prefix: Префикс для файлов модели
            vocab_size: Размер словаря (если None, используется self.vocab_size)
        """
        try:
            import sentencepiece as spm
        except ImportError:
            raise ImportError("Установите sentencepiece: pip install sentencepiece")

        vocab_size = vocab_size or self.vocab_size

        # Если texts - это путь к файлу
        if isinstance(texts, str) and os.path.exists(texts):
            input_file = texts
        else:
            # Сохраняем тексты во временный файл
            input_file = f"{model_prefix}_temp.txt"
            with open(input_file, 'w', encoding='utf-8') as f:
                if isinstance(texts, list):
                    for text in texts:
                        f.write(text + '\n')
                else:
                    f.write(texts)

        # Обучаем SentencePiece
        spm.SentencePieceTrainer.train(
            input=input_file,
            model_prefix=model_prefix,
            vocab_size=vocab_size,
            character_coverage=0.9995,  # Покрытие символов (высокое для русского)
            model_type='bpe',  # Byte-Pair Encoding
            pad_id=0,
            unk_id=1,
            bos_id=2,
            eos_id=3,
            pad_piece=self.pad_token,
            unk_piece=self.unk_token,
            bos_piece=self.bos_token,
            eos_piece=self.eos_token,
            user_defined_symbols=[],
        )

        # Загружаем обученную модель
        self.load(f"{model_prefix}.model")

        # Удаляем временный файл если создавали
        if isinstance(texts, list) and os.path.exists(input_file):
            os.remove(input_file)

        print(f"Токенизатор обучен. Размер словаря: {len(self.vocab)}")

    def load(self, model_file: str):
        """Загрузить обученную модель токенизатора."""
        try:
            import sentencepiece as spm
        except ImportError:
            raise ImportError("Установите sentencepiece: pip install sentencepiece")

        self.sp_model = spm.SentencePieceProcessor()
        self.sp_model.load(model_file)

        # Создаем словари
        self.vocab = {self.sp_model.id_to_piece(i): i for i in range(self.sp_model.get_piece_size())}
        self.reverse_vocab = {i: piece for piece, i in self.vocab.items()}

        self.vocab_size = self.sp_model.get_piece_size()

        # Обновляем специальные токены
        self.pad_token_id = self.sp_model.pad_id()
        self.unk_token_id = self.sp_model.unk_id()
        self.bos_token_id = self.sp_model.bos_id()
        self.eos_token_id = self.sp_model.eos_id()

    def encode(
        self,
        text: str,
        add_bos: bool = True,
        add_eos: bool = True,
    ) -> List[int]:
        """
        Закодировать текст в токены.

        Args:
            text: Входной текст
            add_bos: Добавить токен начала
            add_eos: Добавить токен конца

        Returns:
            Список ID токенов
        """
        if self.sp_model is None:
            raise ValueError("Токенизатор не загружен. Вызовите train() или load() сначала")

        tokens = self.sp_model.encode(text)

        if add_bos:
            tokens = [self.bos_token_id] + tokens
        if add_eos:
            tokens = tokens + [self.eos_token_id]

        return tokens

    def decode(self, tokens: List[int], skip_special_tokens: bool = True) -> str:
        """
        Декодировать токены в текст.

        Args:
            tokens: Список ID токенов
            skip_special_tokens: Пропускать специальные токены

        Returns:
            Декодированный текст
        """
        if self.sp_model is None:
            raise ValueError("Токенизатор не загружен")

        if skip_special_tokens:
            # Удаляем специальные токены
            special_ids = {self.pad_token_id, self.bos_token_id, self.eos_token_id}
            tokens = [t for t in tokens if t not in special_ids]

        return self.sp_model.decode(tokens)

    def tokenize(self, text: str) -> List[str]:
        """Разбить текст на токены (строки)."""
        if self.sp_model is None:
            raise ValueError("Токенизатор не загружен")

        return self.sp_model.encode(text, out_type=str)

    def convert_tokens_to_ids(self, tokens: List[str]) -> List[int]:
        """Конвертировать токены в ID."""
        return [self.vocab.get(token, self.unk_token_id) for token in tokens]

    def convert_ids_to_tokens(self, ids: List[int]) -> List[str]:
        """Конвертировать ID в токены."""
        return [self.reverse_vocab.get(id, self.unk_token) for id in ids]

    def save_pretrained(self, save_directory: str):
        """Сохранить токенизатор."""
        os.makedirs(save_directory, exist_ok=True)

        if self.sp_model is None:
            raise ValueError("Токенизатор не загружен")

        # Сохраняем модель SentencePiece
        model_file = os.path.join(save_directory, "tokenizer.model")
        # Копируем файл модели
        import shutil
        if hasattr(self.sp_model, 'model_file'):
            shutil.copy(self.sp_model.model_file, model_file)

        # Сохраняем конфигурацию
        config = {
            'vocab_size': self.vocab_size,
            'pad_token': self.pad_token,
            'unk_token': self.unk_token,
            'bos_token': self.bos_token,
            'eos_token': self.eos_token,
            'pad_token_id': self.pad_token_id,
            'unk_token_id': self.unk_token_id,
            'bos_token_id': self.bos_token_id,
            'eos_token_id': self.eos_token_id,
        }

        config_file = os.path.join(save_directory, "tokenizer_config.json")
        with open(config_file, 'w', encoding='utf-8') as f:
            json.dump(config, f, indent=2, ensure_ascii=False)

        print(f"Токенизатор сохранен в {save_directory}")

    @classmethod
    def from_pretrained(cls, load_directory: str):
        """Загрузить токенизатор."""
        # Загружаем конфигурацию
        config_file = os.path.join(load_directory, "tokenizer_config.json")
        with open(config_file, 'r', encoding='utf-8') as f:
            config = json.load(f)

        # Создаем токенизатор
        tokenizer = cls(
            vocab_size=config['vocab_size'],
            pad_token=config['pad_token'],
            unk_token=config['unk_token'],
            bos_token=config['bos_token'],
            eos_token=config['eos_token'],
        )

        # Загружаем модель
        model_file = os.path.join(load_directory, "tokenizer.model")
        tokenizer.load(model_file)

        print(f"Токенизатор загружен из {load_directory}")
        return tokenizer

    def __len__(self):
        """Размер словаря."""
        return self.vocab_size

    def __call__(
        self,
        text: Union[str, List[str]],
        padding: bool = False,
        max_length: Optional[int] = None,
        truncation: bool = False,
        return_tensors: Optional[str] = None,
    ):
        """
        Обработка текста (Hugging Face-compatible интерфейс).

        Args:
            text: Текст или список текстов
            padding: Добавить паддинг
            max_length: Максимальная длина
            truncation: Обрезать до max_length
            return_tensors: Формат возврата ('pt' для PyTorch)

        Returns:
            Словарь с input_ids и attention_mask
        """
        # Обработка одного или нескольких текстов
        if isinstance(text, str):
            texts = [text]
        else:
            texts = text

        # Кодируем все тексты
        all_input_ids = [self.encode(t) for t in texts]

        # Truncation
        if truncation and max_length:
            all_input_ids = [ids[:max_length] for ids in all_input_ids]

        # Padding
        if padding:
            if max_length is None:
                max_length = max(len(ids) for ids in all_input_ids)

            padded_ids = []
            attention_masks = []

            for ids in all_input_ids:
                padding_length = max_length - len(ids)
                padded = ids + [self.pad_token_id] * padding_length
                mask = [1] * len(ids) + [0] * padding_length

                padded_ids.append(padded)
                attention_masks.append(mask)

            all_input_ids = padded_ids
        else:
            attention_masks = [[1] * len(ids) for ids in all_input_ids]

        # Формат возврата
        result = {
            'input_ids': all_input_ids,
            'attention_mask': attention_masks,
        }

        if return_tensors == 'pt':
            import torch
            result['input_ids'] = torch.tensor(result['input_ids'])
            result['attention_mask'] = torch.tensor(result['attention_mask'])

        # Если был один текст, возвращаем без батча
        if isinstance(text, str):
            if return_tensors == 'pt':
                result['input_ids'] = result['input_ids'][0]
                result['attention_mask'] = result['attention_mask'][0]
            else:
                result['input_ids'] = result['input_ids'][0]
                result['attention_mask'] = result['attention_mask'][0]

        return result
