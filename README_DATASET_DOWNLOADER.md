# Allan Dataset Downloader

🚀 **Загрузчик и предподготовка датасетов для проекта Allan**

Автоматическое скачивание, предобработка и сохранение русскоязычных датасетов прямо на Google Drive для обучения языковых моделей.

## 🌟 Особенности

- **Автоматическая загрузка** датасетов из Hugging Face Hub и прямых URL
- **Предобработка данных** с настраиваемыми шагами для разных типов задач
- **Валидация качества** загруженных данных
- **Оптимизация для Google Drive** - все данные сохраняются в структурированном виде
- **Поддержка русскоязычных датасетов** для NLP задач
- **Мониторинг ресурсов** и использование диска
- **Прогресс-бары** для отслеживания загрузки

## 📚 Поддерживаемые датасеты

### Hugging Face Hub
- **sberquad** - Русский датасет вопрос-ответ (150 МБ)
- **rucola** - Корпус лингвистической приемлемости (50 МБ)
- **russian_superglue** - Набор задач для оценки (200 МБ)
- **lenta_news** - Новостные статьи Lenta.ru (2 ГБ)
- **russian_poems** - Корпус русской поэзии (150 МБ)

### Прямые URL
- **russian_paraphrase** - Датасет русских парафраз (80 МБ)
- **russian_sentiment** - Анализ тональности (120 МБ)

## 🚀 Быстрый старт

### 1. Установка зависимостей

```bash
pip install requests tqdm psutil
```

### 2. Простая загрузка датасета

```python
from allan_dataset_downloader import quick_download_dataset

# Загрузка с предобработкой
success = quick_download_dataset("sberquad")

# Загрузка только сырых данных
success = quick_download_dataset("russian_paraphrase", skip_preprocessing=True)
```

### 3. Полный контроль над процессом

```python
from allan_dataset_downloader import AllanDatasetDownloader

# Создание загрузчика
downloader = AllanDatasetDownloader()

# Просмотр доступных датасетов
downloader.list_available_datasets()

# Полная загрузка и предобработка
success = downloader.download_and_preprocess("rucola")

# Проверка статуса
status = downloader.get_dataset_status("rucola")
```

## 📁 Структура сохранения

```
/content/drive/MyDrive/ML_Projects/Allan_Model/
├── datasets/
│   ├── raw/           # Сырые загруженные данные
│   ├── processed/     # Предобработанные данные
│   ├── cached/        # Кэш Hugging Face датасетов
│   └── temp/          # Временные файлы
```

## 🔧 Настройка путей

По умолчанию загрузчик использует путь:
```python
project_path = "/content/drive/MyDrive/ML_Projects/Allan_Model"
```

Для изменения пути:
```python
downloader = AllanDatasetDownloader(
    project_path="/path/to/your/project"
)
```

## 📊 Мониторинг и управление

### Проверка статуса датасета
```python
status = downloader.get_dataset_status("dataset_name")
print(f"Сырые данные: {status['raw_exists']}")
print(f"Обработанные: {status['processed_exists']}")
print(f"Размер: {status['raw_size_mb']:.1f} МБ")
```

### Использование диска
```python
disk_usage = downloader.get_disk_usage()
print(f"Всего: {disk_usage['total_gb']:.1f} ГБ")
print(f"Использовано: {disk_usage['used_percent']:.1f}%")
```

### Очистка временных файлов
```python
downloader.cleanup_temp_files()
```

## 🎯 Типы задач и предобработка

### Вопрос-ответ (QA)
- Токенизация
- Обрезка до 512 токенов
- Валидация формата QA

### Классификация
- Токенизация
- Обрезка до 128 токенов
- Бинарные метки

### Генерация текста
- Очистка текста
- Удаление HTML тегов
- Обрезка до 1024 токенов

### Анализ тональности
- Извлечение предложений
- Создание меток
- Обрезка до 256 токенов

## ⚡ Быстрые функции

```python
# Просмотр доступных датасетов
from allan_dataset_downloader import list_downloadable_datasets
list_downloadable_datasets()

# Быстрая загрузка
from allan_dataset_downloader import quick_download_dataset
quick_download_dataset("dataset_name")

# Получение статуса
from allan_dataset_downloader import get_dataset_status
status = get_dataset_status("dataset_name")
```

## 🔍 Валидация данных

Каждый датасет проходит проверки:
- **Формат данных** - соответствие ожидаемой структуре
- **Качество русского текста** - проверка корректности
- **Целостность** - отсутствие поврежденных записей

## 📦 Установка зависимостей

Загрузчик автоматически устанавливает необходимые библиотеки:
- `transformers` и `datasets` для Hugging Face
- `pandas` и `numpy` для обработки данных
- Другие зависимости по требованию

## 🚨 Обработка ошибок

- **Автоматические повторы** при сбоях загрузки
- **Проверка свободного места** перед загрузкой
- **Валидация загруженных файлов**
- **Подробные сообщения об ошибках**

## 💡 Примеры использования

### Загрузка нескольких датасетов
```python
datasets_to_download = ["sberquad", "rucola", "russian_poems"]

for dataset_name in datasets_to_download:
    print(f"Загрузка {dataset_name}...")
    success = downloader.download_and_preprocess(dataset_name)
    if success:
        print(f"✅ {dataset_name} загружен успешно")
    else:
        print(f"❌ Ошибка загрузки {dataset_name}")
```

### Проверка всех загруженных датасетов
```python
available_datasets = downloader.dataset_configs.keys()

for dataset_name in available_datasets:
    status = downloader.get_dataset_status(dataset_name)
    if status['raw_exists'] or status['processed_exists']:
        print(f"📁 {dataset_name}: {'✅' if status['raw_exists'] else '❌'} сырые, {'✅' if status['processed_exists'] else '❌'} обработанные")
```

## 🔧 Расширение функциональности

### Добавление нового датасета
```python
# В методе _load_dataset_configs добавьте:
"new_dataset": DatasetConfig(
    name="new_dataset",
    source_url="path/to/dataset",
    source_type="huggingface",  # или "url"
    format="hf_dataset",         # или "zip", "tar", etc.
    size_mb=100,
    description="Описание датасета",
    task_type="classification",
    preprocessing_steps=["custom_step"],
    validation_checks=["custom_check"]
)
```

### Кастомные шаги предобработки
```python
def _custom_preprocessing_step(self, raw_path: str, processed_path: str):
    """Ваша логика предобработки"""
    # Реализуйте здесь
    pass
```

## 📝 Логирование

Все операции логируются с эмодзи для удобства:
- 📥 Загрузка
- 🔧 Предобработка  
- ✅ Успех
- ❌ Ошибки
- 🔍 Валидация
- 🧹 Очистка

## 🤝 Интеграция с Allan

Загрузчик интегрирован с основным проектом Allan:
- Использует ту же структуру папок
- Совместим с `allan_dataset_manager.py`
- Поддерживает общие настройки проекта

## 📞 Поддержка

При возникновении проблем:
1. Проверьте подключение к Google Drive
2. Убедитесь в достаточном свободном месте
3. Проверьте интернет-соединение
4. Посмотрите на сообщения об ошибках

## 🎉 Заключение

Allan Dataset Downloader предоставляет мощный и удобный способ загрузки русскоязычных датасетов для обучения языковых моделей. Автоматизация процесса загрузки и предобработки экономит время и обеспечивает качество данных.

**Удачного обучения моделей! 🚀**
