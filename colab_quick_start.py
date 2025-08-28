#!/usr/bin/env python3
"""
🚀 Allan Quick Start для Google Colab
Быстрый запуск проекта Allan с минимальными настройками
"""

import os
import subprocess
import sys
from pathlib import Path

def print_header():
    """Печать заголовка"""
    print("=" * 60)
    print("🚀 Allan - Быстрый старт в Google Colab")
    print("=" * 60)

def check_colab():
    """Проверка, что мы в Google Colab"""
    try:
        import google.colab
        print("✅ Google Colab обнаружен")
        return True
    except ImportError:
        print("❌ Этот скрипт предназначен для Google Colab")
        return False

def mount_drive():
    """Подключение Google Drive"""
    print("\n🔗 Подключение Google Drive...")
    try:
        from google.colab import drive
        drive.mount('/content/drive')
        print("✅ Google Drive подключен")
        return True
    except Exception as e:
        print(f"❌ Ошибка подключения Drive: {e}")
        return False

def install_dependencies():
    """Установка зависимостей"""
    print("\n📦 Установка зависимостей...")
    
    packages = [
        "transformers",
        "datasets", 
        "accelerate",
        "peft",
        "trl",
        "bitsandbytes",
        "psutil",
        "pathlib"
    ]
    
    for package in packages:
        try:
            print(f"  📥 Установка {package}...")
            subprocess.run([
                sys.executable, "-m", "pip", "install", "-q", package
            ], check=True, capture_output=True)
            print(f"  ✅ {package} установлен")
        except subprocess.CalledProcessError as e:
            print(f"  ⚠️  Ошибка установки {package}: {e}")

def setup_project_structure():
    """Создание структуры проекта"""
    print("\n📁 Создание структуры проекта...")
    
    base_path = "/content/drive/MyDrive/ML_Projects/Allan_Model"
    directories = [
        f"{base_path}/datasets/raw",
        f"{base_path}/datasets/processed", 
        f"{base_path}/datasets/cached",
        f"{base_path}/models/base",
        f"{base_path}/models/merged",
        f"{base_path}/models/gguf",
        f"{base_path}/checkpoints",
        f"{base_path}/configs"
    ]
    
    for directory in directories:
        try:
            os.makedirs(directory, exist_ok=True)
            print(f"  📂 Создана папка: {directory}")
        except Exception as e:
            print(f"  ⚠️  Ошибка создания {directory}: {e}")

def download_allan_files():
    """Загрузка файлов Allan"""
    print("\n📥 Загрузка файлов Allan...")
    
    # Создаем временную папку для Allan
    allan_path = "/content/allan_temp"
    os.makedirs(allan_path, exist_ok=True)
    
    # Список основных файлов для копирования
    files_to_copy = [
        "allan_dataset_manager.py",
        "allan_train_colab.ipynb", 
        "colab_ru_qlora_gguf.ipynb",
        "allan_dataset_downloader.py",
        "allan_performance_optimizer.py"
    ]
    
    # Копируем файлы из текущей директории
    current_dir = Path.cwd()
    for file_name in files_to_copy:
        source = current_dir / file_name
        if source.exists():
            try:
                import shutil
                shutil.copy2(source, f"{allan_path}/{file_name}")
                print(f"  📄 Скопирован: {file_name}")
            except Exception as e:
                print(f"  ⚠️  Ошибка копирования {file_name}: {e}")
        else:
            print(f"  ⚠️  Файл не найден: {file_name}")
    
    return allan_path

def setup_python_path(allan_path):
    """Настройка Python path"""
    print(f"\n🐍 Настройка Python path...")
    
    if allan_path not in sys.path:
        sys.path.append(allan_path)
        print(f"  ✅ {allan_path} добавлен в Python path")
    
    # Проверяем импорт
    try:
        from allan_dataset_manager import AllanDatasetManager
        print("  ✅ Allan Dataset Manager успешно импортирован")
        return True
    except ImportError as e:
        print(f"  ❌ Ошибка импорта: {e}")
        return False

def test_dataset_manager():
    """Тестирование менеджера датасетов"""
    print("\n🧪 Тестирование Allan Dataset Manager...")
    
    try:
        from allan_dataset_manager import AllanDatasetManager, list_datasets
        
        # Создаем экземпляр менеджера
        manager = AllanDatasetManager()
        print("  ✅ Менеджер создан")
        
        # Показываем доступные датасеты
        print("  📚 Доступные датасеты:")
        list_datasets()
        
        # Мониторинг ресурсов
        print("  🔍 Мониторинг ресурсов:")
        manager.monitor_resources()
        
        return True
        
    except Exception as e:
        print(f"  ❌ Ошибка тестирования: {e}")
        return False

def create_startup_notebook():
    """Создание стартового ноутбука"""
    print("\n📓 Создание стартового ноутбука...")
    
    notebook_content = '''{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# 🚀 Allan - Быстрый старт в Google Colab\\n",
    "\\n",
    "Этот ноутбук создан автоматически для быстрого запуска Allan.\\n",
    "\\n",
    "## 📋 Что делать дальше:\\n",
    "1. Запустите все ячейки по порядку\\n",
    "2. Выберите нужный ноутбук для обучения\\n",
    "3. Настройте параметры под свои задачи\\n",
    "4. Запустите обучение!\\n",
    "\\n",
    "## 📚 Полезные файлы:\\n",
    "- `allan_train_colab.ipynb` - базовое обучение GPT-2\\n",
    "- `colab_ru_qlora_gguf.ipynb` - продвинутое обучение с QLoRA\\n",
    "- `allan_dataset_manager.py` - управление датасетами"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# 🔗 Подключение Google Drive\\n",
    "from google.colab import drive\\n",
    "drive.mount('/content/drive')"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# 📦 Установка зависимостей\\n",
    "!pip install -q transformers datasets accelerate peft trl bitsandbytes psutil"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# 🚀 Импорт Allan\\n",
    "import sys\\n",
    "sys.path.append('/content/allan_temp')\\n",
    "\\n",
    "from allan_dataset_manager import AllanDatasetManager, quick_load_dataset, list_datasets\\n",
    "print('✅ Allan успешно импортирован!')"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# 📊 Просмотр доступных датасетов\\n",
    "list_datasets()"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# 🔍 Проверка ресурсов\\n",
    "manager = AllanDatasetManager()\\n",
    "manager.monitor_resources()"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# 🎯 Быстрая загрузка датасета (пример)\\n",
    "# dataset = quick_load_dataset('sberquad')\\n",
    "# print(f'Датасет загружен: {len(dataset)} примеров')"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "codemirror_mode": {
    "name": "ipython",
    "version": 3
   },
   "file_extension": ".py",
   "mimetype": "text/x-python",
   "name": "python",
   "nbconvert_exporter": "python",
   "pygments_lexer": "ipython3",
   "version": "3.8.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 4
}'''
    
    try:
        with open("/content/allan_quick_start.ipynb", "w", encoding="utf-8") as f:
            f.write(notebook_content)
        print("  ✅ Создан ноутбук: allan_quick_start.ipynb")
        return True
    except Exception as e:
        print(f"  ❌ Ошибка создания ноутбука: {e}")
        return False

def print_next_steps():
    """Печать следующих шагов"""
    print("\n" + "=" * 60)
    print("🎯 Следующие шаги:")
    print("=" * 60)
    print("""
1. 📓 Откройте созданный ноутбук: allan_quick_start.ipynb
2. 🚀 Запустите все ячейки по порядку
3. 📚 Выберите нужный ноутбук для обучения:
   - allan_train_colab.ipynb (базовое обучение GPT-2)
   - colab_ru_qlora_gguf.ipynb (продвинутое обучение с QLoRA)
4. ⚙️ Настройте параметры под свои задачи
5. 🧠 Запустите обучение!
6. 💾 Скачайте результаты с Google Drive

📖 Подробное руководство: COLAB_STARTUP_GUIDE_RU.md
🔧 Allan Dataset Manager готов к использованию!

Удачи с Allan! 🚀
""")

def main():
    """Основная функция"""
    print_header()
    
    if not check_colab():
        return
    
    # Подключение Drive
    if not mount_drive():
        print("❌ Не удалось подключить Drive. Проверьте авторизацию.")
        return
    
    # Установка зависимостей
    install_dependencies()
    
    # Создание структуры проекта
    setup_project_structure()
    
    # Загрузка файлов Allan
    allan_path = download_allan_files()
    
    # Настройка Python path
    if not setup_python_path(allan_path):
        print("❌ Не удалось настроить Allan. Проверьте файлы.")
        return
    
    # Тестирование
    if not test_dataset_manager():
        print("❌ Allan Dataset Manager не работает корректно.")
        return
    
    # Создание стартового ноутбука
    create_startup_notebook()
    
    # Следующие шаги
    print_next_steps()

if __name__ == "__main__":
    main()
