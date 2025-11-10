# Быстрый старт Allan в Google Colab (5 минут)

## Для тех кто спешит!

Если у вас нет времени на загрузку больших датасетов, используйте этот скрипт.

### Шаг 1: Скопируйте эту ячейку в Colab

```python
# 1. Установка зависимостей (1 минута)
!pip install -q torch sentencepiece tqdm

# 2. Клонируем Allan
!git clone https://github.com/KEYTRON/Allan.git
%cd Allan

import sys
sys.path.insert(0, '/content/Allan/src')

# 3. Быстрый датасет и токенизатор (30 секунд)
!wget https://raw.githubusercontent.com/KEYTRON/Allan/main/notebooks/quick_start_colab.py

from quick_start_colab import quick_train_tokenizer, prepare_quick_dataset, ALL_TEXTS
from model.tokenizer import AllanTokenizer
from model.architecture import AllanGPT, AllanConfig

# Обучаем токенизатор на встроенных данных
print("Обучение токенизатора...")
model_file = quick_train_tokenizer(vocab_size=5000)

# Загружаем токенизатор
tokenizer = AllanTokenizer()
tokenizer.load(model_file)

# Готовим данные
train_data = prepare_quick_dataset(tokenizer, max_length=128)

print("\n✓ Подготовка завершена!")
```

### Шаг 2: Создайте и обучите модель (2-3 минуты)

```python
import torch
from torch.utils.data import Dataset, DataLoader
from torch.optim import AdamW
from tqdm import tqdm

# Маленькая модель для быстрого теста
config = AllanConfig(
    vocab_size=5000,
    n_embd=256,      # Маленькая для скорости
    n_layer=4,       # Всего 4 слоя
    n_head=4,
    n_positions=128,
    dropout=0.1,
)

model = AllanGPT(config)
device = 'cuda' if torch.cuda.is_available() else 'cpu'
model = model.to(device)

print(f"Модель создана: {sum(p.numel() for p in model.parameters())/1e6:.1f}M параметров")
print(f"Устройство: {device}")

# Простой датасет
class SimpleDataset(Dataset):
    def __init__(self, data):
        self.data = data
    def __len__(self):
        return len(self.data)
    def __getitem__(self, idx):
        return self.data[idx]

dataset = SimpleDataset(train_data)
dataloader = DataLoader(dataset, batch_size=32, shuffle=True)

# Оптимизатор
optimizer = AdamW(model.parameters(), lr=1e-3)

# Быстрое обучение (2 эпохи для теста)
model.train()
for epoch in range(2):
    total_loss = 0
    pbar = tqdm(dataloader, desc=f"Эпоха {epoch+1}/2")

    for batch in pbar:
        batch = batch.to(device)

        optimizer.zero_grad()
        logits, loss = model(batch, labels=batch)
        loss.backward()
        optimizer.step()

        total_loss += loss.item()
        pbar.set_postfix({'loss': f'{loss.item():.4f}'})

    avg_loss = total_loss / len(dataloader)
    print(f"Эпоха {epoch+1} | Средний loss: {avg_loss:.4f}")

print("\n✓ Обучение завершено!")
```

### Шаг 3: Тестируйте генерацию

```python
# Генерация текста
model.eval()

prompt = "Россия - это"
print(f"Промпт: {prompt}")

input_ids = torch.tensor([tokenizer.encode(prompt)]).to(device)

with torch.no_grad():
    generated = model.generate(
        input_ids,
        max_new_tokens=30,
        temperature=0.8,
        top_k=50
    )

result = tokenizer.decode(generated[0].tolist())
print(f"\nРезультат:\n{result}")
```

### Шаг 4 (опционально): Сохраните на Google Drive

```python
# Монтируем Drive
from google.colab import drive
drive.mount('/content/drive')

# Сохраняем модель
save_path = "/content/drive/MyDrive/allan_quick_test"
model.save_pretrained(save_path)
tokenizer.save_pretrained(save_path)

print(f"✓ Модель сохранена в {save_path}")
```

## Что это даёт?

- **Время**: ~5 минут вместо часов
- **Данные**: Встроенный датасет (8000 текстов)
- **Модель**: Маленькая (~10M параметров)
- **Цель**: Быстро протестировать что всё работает

## Что дальше?

После того как убедитесь что всё работает:

1. Используйте **свой dataset.jsonl** с Drive
2. Увеличьте модель (больше слоев и эмбеддингов)
3. Обучайте дольше (больше эпох)
4. Используйте полный `allan_train_from_scratch.ipynb`

## Если нужно больше данных

Замените quick_train_tokenizer на загрузку из corus:

```python
# Загрузка небольшой части из corus
from datasets.data_loader import CorusDataLoader

loader = CorusDataLoader()
texts = []

# Берём первые 5000 текстов из Lenta.ru
for i, text in enumerate(loader.load_lenta()):
    texts.append(text)
    if i >= 5000:
        break

print(f"Загружено {len(texts)} текстов из Lenta.ru")

# Сохраняем и обучаем токенизатор
temp_file = "/content/lenta_small.txt"
with open(temp_file, 'w', encoding='utf-8') as f:
    for text in texts:
        f.write(text + '\n')

# Обучаем как обычно
import sentencepiece as spm
spm.SentencePieceTrainer.train(
    input=temp_file,
    model_prefix="/content/allan_tokenizer",
    vocab_size=10000
)
```

---

**Теперь можно идти на работу, а модель обучится за 5 минут!** ⚡
