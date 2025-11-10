# Установка Allan

## Требования

- Python >= 3.9
- pip или conda

## Установка из исходников

### 1. Клонирование репозитория

```bash
git clone https://github.com/KEYTRON/Allan.git
cd Allan
```

### 2. Установка зависимостей

#### Базовая установка

```bash
pip install -r requirements.txt
```

#### Установка в режиме разработки

```bash
pip install -e .
```

#### Установка со всеми опциями

```bash
pip install -e ".[all]"
```

### 3. Установка опциональных компонентов

#### Только для русского языка

```bash
pip install -e ".[russian]"
```

#### Для Google Colab

```bash
pip install -e ".[colab]"
```

#### Для мониторинга

```bash
pip install -e ".[monitoring]"
```

#### Для оптимизации (8-bit, 4-bit)

```bash
pip install -e ".[optimization]"
```

#### Для разработки (тесты, линтеры)

```bash
pip install -e ".[dev]"
```

## Конфигурация

### Переменные окружения

Скопируйте `.env.example` в `.env` и настройте:

```bash
cp .env.example .env
```

Отредактируйте `.env`:

```bash
ALLAN_PROJECT_PATH=/path/to/your/project
ALLAN_CACHE_PATH=/path/to/cache
ALLAN_DEVICE=auto  # auto, cpu, cuda, mps
HF_TOKEN=your_huggingface_token
```

### Конфигурационный файл

Создайте `config.yaml`:

```yaml
project_path: /path/to/project
local_cache_path: /path/to/cache
device: auto
small_dataset_threshold: 100
medium_dataset_threshold: 500
large_dataset_threshold: 2000
```

## Проверка установки

```bash
# Запуск Allan
python -m src.main

# Или если установлен как пакет
allan

# Запуск тестов
pytest tests/ -v

# С покрытием
pytest tests/ --cov=src --cov-report=html
```

## Быстрый старт

```python
from src import Allan

# Инициализация
allan = Allan()

# Загрузка модели
allan.load_model("Qwen/Qwen2.5-1.5B-Instruct")

# Генерация текста
result = allan.generate(
    "Привет! Как дела?",
    max_length=100,
    temperature=0.7
)
print(result)
```

## Использование в Google Colab

```python
# Установка в Colab
!git clone https://github.com/KEYTRON/Allan.git
%cd Allan
!pip install -e ".[colab,russian]"

# Монтирование Google Drive
from google.colab import drive
drive.mount('/content/drive')

# Использование
from src import Allan, load_config

config = load_config(
    project_path='/content/drive/MyDrive/ML_Projects/Allan_Model'
)
allan = Allan(config=config)
```

## Решение проблем

### CUDA не найдена

```bash
# Проверка CUDA
python -c "import torch; print(torch.cuda.is_available())"

# Переустановка PyTorch с CUDA
pip install torch --index-url https://download.pytorch.org/whl/cu118
```

### Ошибки импорта

```bash
# Убедитесь что находитесь в корне проекта
export PYTHONPATH="${PYTHONPATH}:$(pwd)"

# Или используйте editable install
pip install -e .
```

### Недостаточно памяти

```bash
# Используйте 8-bit или 4-bit загрузку
allan.load_model("model_name", load_in_8bit=True)
```

## Дополнительная информация

- [README_RU.md](README_RU.md) - Полная документация на русском
- [QUICK_START_RU.md](QUICK_START_RU.md) - Быстрый старт
- [ROADMAP.md](ROADMAP.md) - План развития
