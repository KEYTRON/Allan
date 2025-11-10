# Руководство по обучению Allan в Google Colab

## Обзор

Allan - это фреймворк для создания **собственной** русскоязычной языковой модели с нуля (не дообучение!).

### Что делает Allan:

- Создает нейросеть GPT-архитектуры с нуля
- Обучает токенизатор на русском языке
- Загружает датасеты с Google Drive или библиотеки `corus`
- Сохраняет чекпоинты и модель на Google Drive
- Поддерживает возобновление обучения

## Быстрый старт

### 1. Откройте Colab Notebook

Загрузите `allan_train_from_scratch.ipynb` в Google Colab:

```
https://colab.research.google.com/
```

Или откройте напрямую с Google Drive:
```
Google Drive/Allan_v0.2/allan_train_from_scratch.ipynb
```

### 2. Подготовьте датасет

#### Вариант A: Использовать dataset.jsonl с вашего Drive

У вас уже есть `dataset.jsonl` на Drive. В notebook укажите:

```python
DATA_SOURCE = 'drive'
DRIVE_DATASET_FILE = "Colab Notebooks/dataset.jsonl"
```

Формат dataset.jsonl:
```json
{"text": "Ваш текст здесь"}
{"text": "Еще один текст"}
```

#### Вариант B: Использовать русские корпуса из corus

В notebook укажите:

```python
DATA_SOURCE = 'corus'
```

Allan автоматически загрузит:
- Lenta.ru (новости)
- РИА Новости
- Taiga Proza (проза)
- Taiga Social (соцсети)

### 3. Настройте параметры модели

```python
MODEL_CONFIG = {
    'vocab_size': 50000,
    'n_embd': 512,        # Увеличьте для большей модели
    'n_layer': 8,         # Больше слоев = мощнее модель
    'n_head': 8,
    'n_positions': 1024,  # Длина контекста
}
```

#### Размеры моделей:

| Конфигурация | Параметры | Память GPU | Скорость |
|--------------|-----------|------------|----------|
| Малая (512/8/8) | ~50M | ~4GB | Быстро |
| Средняя (768/12/12) | ~110M | ~8GB | Средне |
| Большая (1024/16/16) | ~200M | ~16GB | Медленно |

### 4. Запустите обучение

Просто выполните все ячейки notebook по порядку:

1. Настройка окружения ✓
2. Монтирование Google Drive ✓
3. Установка зависимостей ✓
4. Загрузка датасетов ✓
5. Обучение токенизатора ✓
6. Создание модели ✓
7. **ОБУЧЕНИЕ** ← Здесь займет время!
8. Сохранение модели ✓
9. Тестирование ✓

## Структура на Google Drive

После обучения на вашем Drive будет:

```
MyDrive/
└── Allan_Model/
    ├── datasets/
    │   └── train_data.pt          # Подготовленные данные
    ├── checkpoints/
    │   ├── checkpoint_epoch0_step1000.pt
    │   ├── checkpoint_epoch1_step2000.pt
    │   └── ...                     # Чекпоинты обучения
    ├── models/
    │   ├── best_model/             # Лучшая модель
    │   └── allan_final/            # Финальная модель
    └── tokenizer/
        ├── tokenizer.model         # Обученный токенизатор
        └── tokenizer_config.json
```

## Возобновление обучения

Если обучение прервалось, загрузите последний чекпоинт:

```python
from model.architecture import AllanGPT, AllanConfig

# Загрузка чекпоинта
checkpoint_path = f"{CHECKPOINTS_PATH}/checkpoint_epoch1_step2000.pt"
epoch, step = load_checkpoint(checkpoint_path, model, optimizer, scheduler)

# Продолжить обучение
# ... (запустите цикл обучения снова)
```

## Использование обученной модели

### В Colab:

```python
from model.architecture import AllanGPT
from model.tokenizer import AllanTokenizer

# Загрузка модели
model = AllanGPT.from_pretrained("/content/drive/MyDrive/Allan_Model/models/allan_final")
tokenizer = AllanTokenizer.from_pretrained("/content/drive/MyDrive/Allan_Model/models/allan_final")

# Генерация
prompt = "Россия - это"
input_ids = torch.tensor([tokenizer.encode(prompt)]).to('cuda')

generated = model.generate(
    input_ids,
    max_new_tokens=100,
    temperature=0.8,
    top_k=50,
    top_p=0.95
)

result = tokenizer.decode(generated[0].tolist())
print(result)
```

### Локально:

1. Скачайте папку модели с Drive
2. Используйте тот же код загрузки

## Параметры обучения

### Основные:

- `batch_size`: Размер батча (уменьшите если не хватает памяти)
- `max_epochs`: Количество эпох обучения
- `learning_rate`: Скорость обучения (3e-4 - хорошее начало)
- `max_seq_length`: Длина последовательности (512 для начала)

### Для ускорения:

- Используйте `gradient_accumulation_steps` для эффективности
- Уменьшите `batch_size` если GPU переполняется
- Уменьшите `n_embd` и `n_layer` для быстрых экспериментов

### Для качества:

- Увеличьте `max_epochs`
- Используйте больше данных
- Увеличьте размер модели (`n_embd`, `n_layer`)

## Мониторинг обучения

В процессе обучения вы увидите:

```
Эпоха 1/10
Training: 100%|██████████| 1000/1000 [10:23<00:00, loss=2.8543]
Средний loss: 2.8543
Perplexity: 17.36
✓ Чекпоинт сохранен
✓ Лучшая модель сохранена
```

**Loss должен уменьшаться**. Если растет - проблема с данными или параметрами.

**Perplexity** - чем меньше, тем лучше модель:
- >100: Модель не обучена
- 20-50: Нормально для начала
- <20: Хорошо
- <10: Отлично

## Советы и трюки

### Нехватка памяти GPU:

```python
# Уменьшите размер модели
MODEL_CONFIG = {
    'n_embd': 384,    # было 512
    'n_layer': 6,     # было 8
}

# Или уменьшите batch_size
TRAINING_CONFIG = {
    'batch_size': 8,  # было 16
    'gradient_accumulation_steps': 8,  # было 4
}
```

### Ускорить обучение:

- Используйте меньше данных для экспериментов
- Уменьшите `max_epochs` для тестов
- Используйте Colab Pro для более мощного GPU

### Улучшить качество:

- Обучайте дольше (больше эпох)
- Используйте больше данных
- Очищайте датасеты от шума
- Экспериментируйте с `learning_rate`

## Библиотека corus

corus предоставляет доступ к русским корпусам:

```python
from datasets.data_loader import CorusDataLoader

loader = CorusDataLoader()

# Lenta.ru
for text in loader.load_lenta():
    print(text)

# РИА
for text in loader.load_ria():
    print(text)

# Taiga
for text in loader.load_taiga('proza'):
    print(text)
```

Доступные корпуса:
- **lenta**: Новости Lenta.ru
- **ria**: РИА Новости
- **taiga_proza**: Художественная проза
- **taiga_social**: Социальные сети
- **taiga_fontanka**: Fontanka.ru
- **wikiner**: WikiNER русский

## Следующие шаги

После обучения базовой модели:

1. **Fine-tuning** - дообучите на специфичных данных
2. **RLHF** - обучение с подкреплением от человека
3. **Квантизация** - сжатие модели для продакшена
4. **Deployment** - развертывание API
5. **Ollama** - конвертация в GGUF для локального использования

## Проблемы и решения

### "Out of memory"
→ Уменьшите `batch_size` или размер модели

### "Loss = NaN"
→ Уменьшите `learning_rate` или проверьте данные

### "Модель генерирует бессмыслицу"
→ Обучайте дольше или используйте больше данных

### "Очень медленно"
→ Используйте меньше данных или меньшую модель для тестов

## Поддержка

- GitHub: https://github.com/KEYTRON/Allan
- Issues: https://github.com/KEYTRON/Allan/issues
- Документация: README_RU.md

---

**Удачного обучения!** 🚀
