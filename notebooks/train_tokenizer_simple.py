"""
Упрощенный скрипт для обучения токенизатора Allan.
Используйте если основной метод не работает.
"""

import sentencepiece as spm
import os


def train_tokenizer_simple(
    texts_file: str,
    model_prefix: str = "allan_tokenizer",
    vocab_size: int = 50000,
):
    """
    Простое обучение токенизатора.

    Args:
        texts_file: Путь к файлу с текстами (одна строка = один текст)
        model_prefix: Префикс для выходных файлов
        vocab_size: Размер словаря
    """

    # Проверяем файл
    if not os.path.exists(texts_file):
        raise FileNotFoundError(f"Файл не найден: {texts_file}")

    file_size = os.path.getsize(texts_file)
    if file_size == 0:
        raise ValueError(f"Файл {texts_file} пустой")

    print(f"Файл: {texts_file}")
    print(f"Размер: {file_size / 1024 / 1024:.2f} MB")

    # Подсчитываем строки
    with open(texts_file, 'r', encoding='utf-8') as f:
        num_lines = sum(1 for line in f if line.strip())

    print(f"Строк с текстом: {num_lines}")

    if num_lines == 0:
        raise ValueError("В файле нет текстов")

    # Обучаем SentencePiece
    print(f"\nОбучение токенизатора (vocab_size={vocab_size})...")

    spm.SentencePieceTrainer.train(
        input=texts_file,
        model_prefix=model_prefix,
        vocab_size=vocab_size,
        character_coverage=0.9995,
        model_type='bpe',
        pad_id=0,
        unk_id=1,
        bos_id=2,
        eos_id=3,
        pad_piece='<PAD>',
        unk_piece='<UNK>',
        bos_piece='<BOS>',
        eos_piece='<EOS>',
        num_threads=os.cpu_count(),
        train_extremely_large_corpus=False,
    )

    print(f"\n✓ Токенизатор обучен!")
    print(f"Модель: {model_prefix}.model")
    print(f"Словарь: {model_prefix}.vocab")

    # Тестируем
    sp = spm.SentencePieceProcessor()
    sp.load(f"{model_prefix}.model")

    test_text = "Привет! Как дела?"
    tokens = sp.encode(test_text)
    decoded = sp.decode(tokens)

    print(f"\nТест:")
    print(f"  Вход: {test_text}")
    print(f"  Токены: {tokens}")
    print(f"  Выход: {decoded}")

    return f"{model_prefix}.model"


if __name__ == "__main__":
    # Пример использования
    import sys

    if len(sys.argv) < 2:
        print("Использование: python train_tokenizer_simple.py <файл_с_текстами>")
        sys.exit(1)

    texts_file = sys.argv[1]
    model_prefix = sys.argv[2] if len(sys.argv) > 2 else "allan_tokenizer"
    vocab_size = int(sys.argv[3]) if len(sys.argv) > 3 else 50000

    train_tokenizer_simple(texts_file, model_prefix, vocab_size)
