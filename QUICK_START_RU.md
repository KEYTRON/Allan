# 🚀 Allan - Быстрый старт в Google Colab

## ⚡ За 5 минут до первого обучения!

### 1️⃣ Откройте Google Colab
- Перейдите на [colab.research.google.com](https://colab.research.google.com)
- Создайте новый ноутбук

### 2️⃣ Скопируйте и вставьте этот код:
```python
# 🚀 Allan - Автоматическая настройка
!wget -q https://raw.githubusercontent.com/your-username/Allan/main/colab_quick_start.py
!python colab_quick_start.py
```

### 3️⃣ Следуйте инструкциям
- Подключите Google Drive
- Дождитесь установки зависимостей
- Откройте созданный ноутбук `allan_quick_start.ipynb`

### 4️⃣ Выберите ноутбук для обучения:

#### 🎯 **Для начинающих** (GPT-2):
```python
# Откройте: allan_train_colab.ipynb
# Простое обучение GPT-2 на русском
```

#### 🚀 **Для продвинутых** (QLoRA + GGUF):
```python
# Откройте: colab_ru_qlora_gguf.ipynb
# Современное обучение с экспортом в Ollama
```

---

## 📚 Что у вас будет:

✅ **Allan Dataset Manager** - умная загрузка датасетов  
✅ **Готовые ноутбуки** для обучения  
✅ **Структура папок** на Google Drive  
✅ **Рекомендованные датасеты** для русского языка  
✅ **Автоматическое сохранение** результатов  

---

## 🎯 Первые шаги после настройки:

1. **Загрузите датасет:**
```python
from allan_dataset_manager import quick_load_dataset
dataset = quick_load_dataset("sberquad")  # Русский QA
```

2. **Проверьте ресурсы:**
```python
manager = AllanDatasetManager()
manager.monitor_resources()
```

3. **Запустите обучение** в выбранном ноутбуке!

---

## 📖 Подробное руководство:
- **COLAB_STARTUP_GUIDE_RU.md** - полное руководство
- **README_RU.md** - документация проекта
- **colab_ru_qlora_gguf.ipynb** - примеры кода

---

**🚀 Allan готов к работе! Удачи с обучением!**
