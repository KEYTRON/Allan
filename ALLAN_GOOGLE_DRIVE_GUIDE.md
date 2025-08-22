# 🔥 Allan Model - Полное руководство по использованию Google Drive

**Максимальная эффективность обучения русскоязычной модели Allan в Google Colab с использованием Google Drive**

## 📋 Содержание

1. [Быстрый старт](#быстрый-старт)
2. [Архитектура системы](#архитектура-системы)
3. [Подробные инструкции](#подробные-инструкции)
4. [Оптимизация производительности](#оптимизация-производительности)
5. [Работа с датасетами](#работа-с-датасетами)
6. [Мониторинг и отладка](#мониторинг-и-отладка)
7. [Лучшие практики](#лучшие-практики)
8. [Решение проблем](#решение-проблем)

---

## 🚀 Быстрый старт

### Шаг 1: Подключение к Colab
```python
# В новом Colab ноутбуке выполните:
!wget https://raw.githubusercontent.com/your-repo/allan_colab_setup.py
!wget https://raw.githubusercontent.com/your-repo/allan_dataset_manager.py
!wget https://raw.githubusercontent.com/your-repo/allan_performance_optimizer.py

# Импорт и запуск автоматической настройки
from allan_colab_setup import setup_allan
setup_allan()
```

### Шаг 2: Загрузка первого датасета
```python
from allan_dataset_manager import quick_load_dataset

# Загрузка рекомендованного датасета
dataset = quick_load_dataset("sberquad")
print(f"Датасет загружен: {len(dataset['train'])} примеров")
```

### Шаг 3: Оптимизация для обучения
```python
from allan_performance_optimizer import optimize_allan_for_training

# Автоматическая оптимизация системы
optimize_allan_for_training()
```

**🎉 Готово! Система настроена и готова к обучению Allan.**

---

## 🏗️ Архитектура системы

### Преимущества Google Drive подхода

| Аспект | Google Drive | Локальное хранилище Colab |
|--------|--------------|----------------------------|
| **Объем** | 2 ТБ (1.99 ТБ свободно) | 80 ГБ |
| **Постоянство** | ✅ Между сессиями | ❌ Удаляется при перезапуске |
| **Скорость** | Средняя (оптимизируемая) | Высокая |
| **Надежность** | ✅ Резервное копирование | ❌ Временное |

### Структура проекта Allan

```
/content/drive/MyDrive/ML_Projects/Allan_Model/
├── 📁 datasets/           # Датасеты (raw, processed, cached)
├── 🤖 models/            # Модели и чекпоинты
├── ⚙️  configs/          # Конфигурационные файлы
├── 📜 scripts/           # Скрипты обучения
├── 📓 notebooks/         # Jupyter ноутбуки
├── 📊 logs/              # Логи (TensorBoard, WandB)
├── 📈 results/           # Результаты экспериментов
├── 💾 cache/             # Кэши (HuggingFace, PyTorch)
├── 📚 docs/              # Документация
└── 🛠️  tools/            # Утилиты и инструменты
```

---

## 📖 Подробные инструкции

### Автоматическая настройка среды

**allan_colab_setup.py** - Главный скрипт настройки:

```python
from allan_colab_setup import AllanColabSetup

setup = AllanColabSetup()

# Пошаговая настройка
setup.mount_drive()              # Подключение Google Drive
setup.install_dependencies()    # Установка библиотек
setup.create_project_structure() # Создание папок
setup.setup_environment()       # Настройка переменных окружения
setup.verify_setup()            # Проверка корректности

# Или все сразу
setup.setup_allan_colab()
```

**Что происходит при настройке:**
- 🔗 Подключение Google Drive с автоматической авторизацией
- 📦 Установка 15+ специализированных библиотек для русского NLP
- 📁 Создание 50+ папок с описаниями в README
- ⚙️ Настройка переменных окружения для кэшей
- 🔍 Проверка GPU, RAM и дискового пространства

### Умное управление датасетами

**allan_dataset_manager.py** - Интеллектуальная загрузка:

```python
from allan_dataset_manager import AllanDatasetManager

manager = AllanDatasetManager()

# Просмотр доступных датасетов
manager.list_available_datasets()

# Универсальная загрузка с автоматическим выбором стратегии
dataset = manager.load_dataset("sberquad")

# Получение статистики
manager.get_dataset_stats(dataset, "SberQuAD")

# Мониторинг ресурсов
manager.monitor_resources()
```

**Стратегии загрузки:**

| Размер датасета | Стратегия | Описание |
|----------------|-----------|----------|
| < 100 МБ | `direct` | Прямое чтение с Drive |
| 100-500 МБ | `copy_local` | Копирование в локальный кэш |
| 500 МБ - 2 ГБ | `copy_local` | Локальное копирование |
| > 2 ГБ | `streaming` | Потоковая загрузка |

---

## ⚡ Оптимизация производительности

### Автоматическая оптимизация

**allan_performance_optimizer.py** - Комплексная оптимизация:

```python
from allan_performance_optimizer import AllanPerformanceOptimizer

optimizer = AllanPerformanceOptimizer()

# Текущее состояние ресурсов
optimizer.print_current_status()

# Оптимизация для обучения
optimizer.optimize_for_training()

# Мониторинг во время обучения (60 минут)
optimizer.monitor_training(duration_minutes=60)

# Генерация отчета
report = optimizer.generate_performance_report()
print(report)
```

### Критические оптимизации

**Переменные окружения:**
```python
# PyTorch оптимизации
os.environ["PYTORCH_CUDA_ALLOC_CONF"] = "max_split_size_mb:128"
os.environ["CUDA_LAUNCH_BLOCKING"] = "0"

# Transformers оптимизации  
os.environ["TRANSFORMERS_NO_ADVISORY_WARNINGS"] = "1"
os.environ["TOKENIZERS_PARALLELISM"] = "true"

# Datasets оптимизации
os.environ["HF_DATASETS_IN_MEMORY_MAX_SIZE"] = "0"
```

**GPU настройки:**
```python
import torch

# Максимальная производительность
torch.backends.cudnn.benchmark = True
torch.backends.cudnn.enabled = True

# Очистка кэша
torch.cuda.empty_cache()
```

---

## 📊 Работа с датасетами

### Рекомендованные датасеты для Allan

| Датасет | Размер | Задача | Описание |
|---------|--------|--------|----------|
| **SberQuAD** | 150 МБ | QA | Русский вопрос-ответ |
| **RuCoLA** | 50 МБ | Classification | Лингвистическая приемлемость |
| **Russian SuperGLUE** | 200 МБ | Multi-task | Комплексная оценка |
| **Lenta News** | 2 ГБ | Generation | Генерация новостей |
| **Russian Poems** | 150 МБ | Generation | Поэзия и стиль |

### Загрузка и предобработка

```python
# Загрузка с автоматической стратегией
dataset = manager.load_dataset("sberquad")

# Предобработка
def preprocess_function(examples):
    # Ваша логика предобработки
    return examples

processed_dataset = dataset.map(preprocess_function, batched=True)

# Сохранение на Drive для переиспользования
processed_dataset.save_to_disk(
    "/content/drive/MyDrive/ML_Projects/Allan_Model/datasets/processed/sberquad_processed"
)
```

### Работа с большими датасетами

```python
# Потоковая загрузка для больших датасетов
large_dataset = manager.load_dataset("lenta_news", streaming=True)

# Работа с потоком
for batch in large_dataset.iter(batch_size=1000):
    # Обработка батча
    process_batch(batch)
```

---

## 🔍 Мониторинг и отладка

### Автоматический мониторинг

```python
# Запуск фонового мониторинга
optimizer.monitor_training(duration_minutes=120, check_interval=30)

# Проверка текущих ресурсов
metrics = optimizer.get_current_metrics()
print(f"RAM: {metrics.ram_percent:.1f}%")
print(f"GPU: {metrics.gpu_percent:.1f}%")
```

### Предупреждения и критические ситуации

**Пороговые значения:**
- ⚠️ **Предупреждение**: RAM > 85%, GPU > 95%, Диск > 90%
- 🚨 **Критично**: RAM > 95%, GPU > 98%, Диск > 95%

**Автоматические действия:**
```python
# При критических проблемах автоматически:
optimizer.auto_cleanup_on_critical()
# - Очистка Python GC
# - Очистка GPU кэша  
# - Удаление временных файлов
# - Очистка pip/apt кэшей
```

### Сохранение метрик на Drive

```python
# Сохранение истории мониторинга
optimizer.save_metrics_to_drive(
    "/content/drive/MyDrive/ML_Projects/Allan_Model/logs/"
)
```

---

## 🎯 Лучшие практики

### Оптимальные параметры обучения

**На основе анализа ресурсов Colab:**

```python
# Для GPU 15-16 ГБ (T4/V100)
training_args = TrainingArguments(
    output_dir="/content/drive/MyDrive/ML_Projects/Allan_Model/models/checkpoints",
    
    # Batch размеры
    per_device_train_batch_size=8,      # Или 16 для больших GPU
    per_device_eval_batch_size=16,
    gradient_accumulation_steps=4,       # Эмуляция batch_size=32
    
    # Оптимизации памяти
    fp16=True,                          # Обязательно для экономии памяти
    gradient_checkpointing=True,        # Экономия GPU памяти
    dataloader_pin_memory=False,        # Экономия RAM
    
    # Сохранение на Drive
    save_strategy="steps",
    save_steps=500,
    save_total_limit=3,                 # Только 3 последних чекпоинта
    
    # Логирование
    logging_dir="/content/drive/MyDrive/ML_Projects/Allan_Model/logs/tensorboard",
    logging_steps=100,
    
    # Оценка
    evaluation_strategy="steps",
    eval_steps=500,
    
    # Оптимизатор
    optim="adamw_torch",
    learning_rate=5e-5,
    warmup_steps=500,
)
```

### Эффективное использование Drive пространства

**Структурированное хранение:**
```python
# Чекпоинты - только лучшие
best_model_path = "/content/drive/MyDrive/ML_Projects/Allan_Model/models/final/allan_v1"

# Логи - с ротацией
log_rotation = {
    "keep_last_n_runs": 5,
    "compress_old_logs": True
}

# Датасеты - кэшированные версии
cache_processed_datasets = True
```

### Мониторинг прогресса

```python
# Интеграция с Weights & Biases
import wandb

wandb.init(
    project="allan-model",
    dir="/content/drive/MyDrive/ML_Projects/Allan_Model/logs/wandb"
)

# Кастомные метрики
def log_custom_metrics():
    metrics = optimizer.get_current_metrics()
    wandb.log({
        "system/ram_usage": metrics.ram_percent,
        "system/gpu_usage": metrics.gpu_percent,
        "system/disk_usage": metrics.disk_percent,
    })
```

---

## 🛠️ Решение проблем

### Частые проблемы и решения

#### 1. "Недостаточно памяти GPU"
```python
# Решение:
torch.cuda.empty_cache()
optimizer.optimize_memory(aggressive=True)

# Уменьшите batch_size
training_args.per_device_train_batch_size = 4
training_args.gradient_accumulation_steps = 8
```

#### 2. "Медленная загрузка с Drive"
```python
# Решение: принудительное локальное копирование
manager = AllanDatasetManager()
dataset = manager.load_dataset_local_copy(
    dataset_path="path/to/dataset",
    dataset_name="my_dataset"
)
```

#### 3. "Переполнение диска"
```python
# Решение: агрессивная очистка
optimizer.optimize_disk_space()

# Удаление старых чекпоинтов
cleanup_old_checkpoints(keep_last=2)
```

#### 4. "Потеря подключения к Drive"
```python
# Решение: переподключение
from google.colab import drive
drive.mount('/content/drive', force_remount=True)

# Проверка доступности
assert os.path.exists('/content/drive/MyDrive')
```

### Диагностика проблем

```python
# Полная диагностика системы
def diagnose_allan_system():
    print("🔍 Диагностика системы Allan...")
    
    # Проверка подключений
    drive_connected = os.path.exists('/content/drive/MyDrive')
    print(f"Drive подключен: {drive_connected}")
    
    # Проверка ресурсов
    optimizer = AllanPerformanceOptimizer()
    optimizer.print_current_status()
    
    # Проверка библиотек
    try:
        import torch, transformers, datasets
        print(f"✅ Библиотеки: PyTorch {torch.__version__}, Transformers {transformers.__version__}")
    except ImportError as e:
        print(f"❌ Ошибка импорта: {e}")
    
    # Проверка GPU
    if torch.cuda.is_available():
        print(f"✅ GPU: {torch.cuda.get_device_name(0)}")
    else:
        print("❌ GPU недоступен")

# Запуск диагностики
diagnose_allan_system()
```

---

## 📚 Дополнительные ресурсы

### Полезные команды

```python
# Быстрая настройка в одну строку
exec(open('allan_colab_setup.py').read()); setup_allan()

# Быстрая загрузка датасета
dataset = quick_load_dataset("sberquad")

# Быстрая оптимизация
optimize_allan_for_training()

# Быстрая очистка
cleanup_allan_resources()
```

### Интеграция с другими инструментами

```python
# Jupyter расширения
!pip install jupyter-tensorboard jupyter-resource-usage

# Системный мониторинг
!pip install gpustat htop

# Визуализация
!pip install plotly dash streamlit
```

### Backup и восстановление

```python
# Создание backup конфигурации
def backup_allan_config():
    backup_data = {
        "project_structure": get_project_structure(),
        "environment_vars": dict(os.environ),
        "installed_packages": get_installed_packages(),
        "system_metrics": optimizer.get_current_metrics()
    }
    
    with open(f"{project_path}/backup_{datetime.now().strftime('%Y%m%d')}.json", 'w') as f:
        json.dump(backup_data, f, indent=2, default=str)
```

---

## 🎉 Заключение

Система Allan + Google Drive предоставляет:

- ✅ **2 ТБ постоянного хранилища** vs 80 ГБ временного
- ✅ **Автоматическую оптимизацию** для максимальной производительности  
- ✅ **Умное управление датасетами** с выбором стратегии загрузки
- ✅ **Непрерывный мониторинг** ресурсов с автоматической очисткой
- ✅ **Структурированную организацию** проекта
- ✅ **Максимальное использование** возможностей Colab

**Результат: Эффективное обучение Allan с минимальными затратами времени на инфраструктуру!**

---

*Создано для проекта Allan Model - русскоязычная языковая модель нового поколения* 🚀