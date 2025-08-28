# 🚀 Allan в Google Colab - Полное руководство по запуску

## 📋 Содержание
1. [Подготовка Google Colab](#подготовка-google-colab)
2. [Загрузка проекта](#загрузка-проекта)
3. [Настройка окружения](#настройка-окружения)
4. [Работа с датасетами](#работа-с-датасетами)
5. [Обучение модели](#обучение-модели)
6. [Экспорт и использование](#экспорт-и-использование)
7. [Что делать с файлами](#что-делать-с-файлами)

---

## 🔧 Подготовка Google Colab

### Шаг 1: Открытие Google Colab
1. Перейдите на [colab.research.google.com](https://colab.research.google.com)
2. Войдите в свой Google аккаунт
3. Создайте новый ноутбук или откройте существующий

### Шаг 2: Подключение Google Drive
```python
# 🔗 Подключение Google Drive
from google.colab import drive
drive.mount('/content/drive')
```
- Следуйте инструкциям для авторизации
- Drive будет доступен по пути `/content/drive/MyDrive`

---

## 📥 Загрузка проекта

### Вариант 1: Клонирование из GitHub
```python
# 📥 Клонирование репозитория Allan
!git clone https://github.com/your-username/Allan.git
%cd Allan
```

### Вариант 2: Загрузка файлов вручную
1. Скачайте файлы проекта на свой компьютер
2. Загрузите их в Google Drive в папку `ML_Projects/Allan_Model`
3. Или используйте загрузку через Colab:
```python
# 📁 Создание структуры папок
!mkdir -p /content/drive/MyDrive/ML_Projects/Allan_Model
!mkdir -p /content/drive/MyDrive/ML_Projects/Allan_Model/datasets
!mkdir -p /content/drive/MyDrive/ML_Projects/Allan_Model/models
!mkdir -p /content/drive/MyDrive/ML_Projects/Allan_Model/checkpoints
```

---

## ⚙️ Настройка окружения

### Установка зависимостей
```python
# 📦 Установка основных библиотек
!pip install -q transformers datasets accelerate peft trl bitsandbytes

# 🔧 Дополнительные библиотеки для Allan
!pip install -q psutil pathlib
```

### Импорт Allan Dataset Manager
```python
# 📚 Импорт менеджера датасетов Allan
import sys
sys.path.append('/content/Allan')  # Если клонировали
# или
sys.path.append('/content/drive/MyDrive/ML_Projects/Allan_Model')  # Если загрузили в Drive

from allan_dataset_manager import AllanDatasetManager, quick_load_dataset, list_datasets
```

---

## 📊 Работа с датасетами

### Просмотр доступных датасетов
```python
# 📚 Просмотр рекомендованных датасетов
list_datasets()
```

### Загрузка датасета
```python
# 🚀 Быстрая загрузка рекомендованного датасета
dataset = quick_load_dataset("sberquad")  # Русский QA датасет

# 📊 Получение статистики
manager = AllanDatasetManager()
manager.get_dataset_stats(dataset, "sberquad")
```

### Загрузка собственного датасета
```python
# 📁 Загрузка JSONL файла с Google Drive
dataset_path = "/content/drive/MyDrive/ML_Projects/Allan_Model/datasets/my_dataset.jsonl"
dataset = manager.load_dataset(dataset_path)

# 🤗 Загрузка с Hugging Face
dataset = manager.load_dataset("IlyaGusev/saiga_qa")
```

---

## 🧠 Обучение модели

### Выбор ноутбука для обучения

#### Для базового обучения (GPT-2):
```python
# 📖 Откройте файл: allan_train_colab.ipynb
# Этот ноутбук подходит для:
# - Обучения GPT-2 на русском
# - Работы с небольшими датасетами
# - Базового понимания процесса
```

#### Для продвинутого обучения (QLoRA + GGUF):
```python
# 🚀 Откройте файл: colab_ru_qlora_gguf.ipynb
# Этот ноутбук подходит для:
# - Дообучения современных моделей (Qwen2.5)
# - Использования QLoRA для экономии памяти
# - Экспорта в GGUF для Ollama
```

### Настройка параметров обучения
```python
# ⚙️ Основные параметры в colab_ru_qlora_gguf.ipynb
train_config = {
    "max_seq_length": 2048,        # Длина последовательности
    "batch_size": 2,               # Размер батча
    "gradient_accumulation_steps": 4,  # Накопление градиентов
    "learning_rate": 2e-4,         # Скорость обучения
    "num_train_epochs": 3,         # Количество эпох
    "save_steps": 500,             # Сохранение каждые N шагов
    "eval_steps": 500,             # Оценка каждые N шагов
}
```

---

## 💾 Экспорт и использование

### Сохранение модели
```python
# 💾 Модель автоматически сохраняется в Google Drive:
# - LoRA адаптеры: /content/drive/MyDrive/ML_Projects/Allan_Model/checkpoints/lora_adapters
# - Полная модель: /content/drive/MyDrive/ML_Projects/Allan_Model/models/merged
# - GGUF файл: /content/drive/MyDrive/ML_Projects/Allan_Model/models/gguf
```

### Скачивание на локальный компьютер
1. Откройте Google Drive в браузере
2. Перейдите в папку `ML_Projects/Allan_Model`
3. Скачайте нужные файлы:
   - `*.gguf` - для Ollama
   - `lora_adapters/` - для продолжения обучения
   - `merged/` - полная модель для Hugging Face

---

## 📁 Что делать с файлами

### Структура проекта Allan
```
ML_Projects/Allan_Model/
├── datasets/           # 📊 Датасеты
│   ├── raw/           # Сырые данные
│   ├── processed/     # Обработанные данные
│   └── cached/        # Кэшированные датасеты
├── models/            # 🤖 Модели
│   ├── base/          # Базовые модели
│   ├── merged/        # Объединенные модели
│   └── gguf/          # GGUF файлы для Ollama
├── checkpoints/       # 💾 Чекпоинты обучения
│   └── lora_adapters/ # LoRA адаптеры
└── configs/           # ⚙️ Конфигурации
```

### Файлы для разных задач

#### 🎯 Для обучения:
- `allan_train_colab.ipynb` - базовое обучение GPT-2
- `colab_ru_qlora_gguf.ipynb` - продвинутое обучение с QLoRA
- `allan_dataset_manager.py` - управление датасетами

#### 📊 Для работы с данными:
- `allan_dataset_downloader.py` - загрузка датасетов
- `allan_dataset_manager.py` - умное управление датасетами
- `example_dataset_download.py` - примеры использования

#### 🚀 Для оптимизации:
- `allan_performance_optimizer.py` - оптимизация производительности
- `allan_colab_setup.py` - настройка Colab окружения

---

## 🔄 Типичный рабочий процесс

### 1. Подготовка (5 минут)
```python
# 🔗 Подключение Drive
from google.colab import drive
drive.mount('/content/drive')

# 📦 Установка зависимостей
!pip install -q transformers datasets accelerate peft trl bitsandbytes
```

### 2. Загрузка проекта (2 минуты)
```python
# 📥 Клонирование или загрузка файлов
!git clone https://github.com/your-username/Allan.git
%cd Allan
```

### 3. Настройка датасета (5-10 минут)
```python
# 📊 Загрузка и проверка датасета
from allan_dataset_manager import quick_load_dataset
dataset = quick_load_dataset("sberquad")
```

### 4. Обучение (30 минут - несколько часов)
```python
# 🧠 Запуск обучения в colab_ru_qlora_gguf.ipynb
# Модель автоматически сохраняется на Drive
```

### 5. Экспорт (10 минут)
```python
# 💾 Автоматическая конвертация в GGUF
# Скачивание файлов с Drive
```

---

## ⚠️ Важные советы

### 💾 Управление памятью
- Используйте `gradient_accumulation_steps` для экономии памяти
- При OOM ошибках уменьшайте `max_seq_length` и `batch_size`
- Используйте QLoRA для больших моделей

### 🔄 Возобновление обучения
- Чекпоинты автоматически сохраняются на Drive
- При перезапуске Colab обучение продолжится с последней точки
- Не теряйте прогресс - все сохраняется в облаке

### 📊 Мониторинг ресурсов
```python
# 🔍 Проверка доступных ресурсов
manager = AllanDatasetManager()
manager.monitor_resources()
```

---

## 🆘 Решение проблем

### Проблема: "Out of Memory"
```python
# 💡 Решения:
# 1. Уменьшите batch_size
# 2. Увеличьте gradient_accumulation_steps
# 3. Уменьшите max_seq_length
# 4. Используйте QLoRA (4-бит квантизация)
```

### Проблема: "Drive не подключается"
```python
# 💡 Решения:
# 1. Проверьте авторизацию в браузере
# 2. Перезапустите Colab
# 3. Используйте прямой путь к файлам
```

### Проблема: "Модель не загружается"
```python
# 💡 Решения:
# 1. Проверьте правильность пути
# 2. Убедитесь, что файлы загружены в Drive
# 3. Проверьте формат датасета (JSONL)
```

---

## 🎯 Следующие шаги

После успешного запуска Allan в Colab:

1. **Изучите результаты обучения** - проверьте качество модели
2. **Экспериментируйте с параметрами** - настройте под свои задачи
3. **Попробуйте разные датасеты** - используйте Allan Dataset Manager
4. **Экспортируйте в GGUF** - для локального использования в Ollama
5. **Поделитесь результатами** - с сообществом Allan

---

## 📞 Поддержка

Если у вас возникли вопросы:
- Проверьте [README_RU.md](README_RU.md) для детальной информации
- Изучите [ROADMAP.md](ROADMAP.md) для понимания развития проекта
- Создайте issue в GitHub репозитории

**Удачи с Allan! 🚀**
