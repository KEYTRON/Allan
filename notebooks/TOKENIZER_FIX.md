# Исправление ошибки обучения токенизатора

## Проблема

```
RuntimeError: Internal: src/trainer_interface.cc(433) [!sentences_.empty()]
```

Это означает, что SentencePiece не получает данных для обучения.

## Решение 1: Используйте упрощенный скрипт

Вместо стандартной ячейки используйте:

```python
# Скачайте упрощенный скрипт
!wget https://raw.githubusercontent.com/KEYTRON/Allan/main/notebooks/train_tokenizer_simple.py

# Проверьте что файл с текстами существует и не пустой
!ls -lh /content/tokenizer_train_texts.txt
!head -5 /content/tokenizer_train_texts.txt

# Обучите токенизатор
from train_tokenizer_simple import train_tokenizer_simple

model_file = train_tokenizer_simple(
    texts_file="/content/tokenizer_train_texts.txt",
    model_prefix="/content/allan_tokenizer",
    vocab_size=50000
)

print(f"✓ Модель: {model_file}")
```

## Решение 2: Проверьте файл с текстами

```python
# Проверьте что файл создан
import os
file_path = "/content/tokenizer_train_texts.txt"

if os.path.exists(file_path):
    size = os.path.getsize(file_path)
    print(f"Размер файла: {size / 1024 / 1024:.2f} MB")

    # Посмотрите первые строки
    with open(file_path, 'r', encoding='utf-8') as f:
        for i, line in enumerate(f):
            if i >= 5:
                break
            print(f"Строка {i+1}: {line[:100]}")
else:
    print("❌ Файл не существует!")
```

## Решение 3: Пересоздайте файл

```python
# Убедитесь что тексты не пустые
print(f"Количество текстов: {len(texts_for_tokenizer)}")
print(f"Первый текст: {texts_for_tokenizer[0][:200]}")

# Пересоздайте файл
temp_file = "/content/tokenizer_train_texts.txt"
with open(temp_file, 'w', encoding='utf-8') as f:
    count = 0
    for text in texts_for_tokenizer[:50000]:
        if text and text.strip():  # Только непустые
            f.write(text.strip() + '\n')
            count += 1

print(f"Записано текстов: {count}")

# Проверьте размер
size = os.path.getsize(temp_file)
print(f"Размер файла: {size / 1024 / 1024:.2f} MB")

# Если размер > 0, обучайте
if size > 0:
    from train_tokenizer_simple import train_tokenizer_simple
    model_file = train_tokenizer_simple(temp_file)
```

## Решение 4: Используйте меньше данных для теста

```python
# Начните с малого количества для проверки
test_texts = [
    "Это тестовый текст для обучения токенизатора.",
    "Привет! Как дела?",
    "Россия - большая страна.",
    "Машинное обучение - интересная область.",
    "Токенизация - важный этап подготовки данных."
] * 1000  # Повторяем для получения достаточного объема

temp_file = "/content/test_tokenizer.txt"
with open(temp_file, 'w', encoding='utf-8') as f:
    for text in test_texts:
        f.write(text + '\n')

print(f"Создано {len(test_texts)} тестовых текстов")

# Обучаем
from train_tokenizer_simple import train_tokenizer_simple
model_file = train_tokenizer_simple(
    texts_file=temp_file,
    vocab_size=5000  # Меньший словарь для теста
)
```

## Решение 5: Обновите Allan

```python
# Обновите код Allan до последней версии
%cd /content/Allan
!git pull origin main

# Переустановите
import sys
sys.path.insert(0, '/content/Allan/src')

# Импортируйте заново
from model.tokenizer import AllanTokenizer
```

## Проверка после обучения

```python
# Загрузите обученный токенизатор
from model.tokenizer import AllanTokenizer

tokenizer = AllanTokenizer()
tokenizer.load("/content/allan_tokenizer.model")

# Тестируйте
test_text = "Россия - это страна с богатой историей."
tokens = tokenizer.encode(test_text)
decoded = tokenizer.decode(tokens)

print(f"Исходный текст: {test_text}")
print(f"Токены: {tokens}")
print(f"Декодированный: {decoded}")
print(f"Размер словаря: {len(tokenizer)}")
```

## Если ничего не помогает

Создайте issue на GitHub с подробным описанием:

1. Версия Python
2. Версия sentencepiece
3. Размер файла с текстами
4. Первые несколько строк файла
5. Полный текст ошибки

```
https://github.com/KEYTRON/Allan/issues
```
