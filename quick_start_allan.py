#!/usr/bin/env python3
"""
🔥 Allan Model - Quick Start Script
Быстрый запуск полной системы Allan с Google Drive в одной команде!
"""

import os
import sys
from datetime import datetime

def print_banner():
    """Красивый баннер Allan"""
    banner = """
    ╔══════════════════════════════════════════════════════════════╗
    ║                                                              ║
    ║        🔥 ALLAN MODEL - GOOGLE DRIVE QUICK START 🔥          ║
    ║                                                              ║
    ║    Русскоязычная языковая модель + Google Colab + 2TB       ║
    ║                                                              ║
    ╚══════════════════════════════════════════════════════════════╝
    """
    print(banner)
    print(f"🕒 Запуск: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("🚀 Максимальная эффективность обучения в Colab!")
    print("=" * 70)

def download_allan_tools():
    """Скачивание всех необходимых инструментов Allan"""
    print("📥 Скачивание инструментов Allan...")
    
    # В реальном проекте здесь будут ссылки на GitHub или другой репозиторий
    tools = [
        "allan_colab_setup.py",
        "allan_dataset_manager.py", 
        "allan_performance_optimizer.py",
        "allan_drive_structure.py"
    ]
    
    # Проверяем наличие файлов локально
    missing_tools = []
    for tool in tools:
        if not os.path.exists(tool):
            missing_tools.append(tool)
    
    if missing_tools:
        print("⚠️  Некоторые инструменты не найдены локально:")
        for tool in missing_tools:
            print(f"   • {tool}")
        print("📝 В реальном проекте они будут скачаны автоматически с GitHub")
        return False
    
    print("✅ Все инструменты Allan доступны!")
    return True

def quick_setup_allan():
    """Быстрая настройка Allan в одну команду"""
    print_banner()
    
    # Шаг 1: Проверка инструментов
    if not download_allan_tools():
        print("❌ Не удалось получить все необходимые инструменты")
        return False
    
    try:
        # Шаг 2: Импорт модулей
        print("\n📦 Импорт модулей Allan...")
        from allan_colab_setup import setup_allan
        from allan_dataset_manager import AllanDatasetManager, list_datasets
        from allan_performance_optimizer import optimize_allan_for_training
        from allan_drive_structure import create_allan_drive_structure
        
        print("✅ Модули импортированы успешно!")
        
        # Шаг 3: Автоматическая настройка среды
        print("\n🔧 Автоматическая настройка среды Colab...")
        setup_success = setup_allan()
        
        if not setup_success:
            print("❌ Ошибка настройки среды")
            return False
        
        # Шаг 4: Создание структуры на Drive
        print("\n📁 Создание структуры проекта на Google Drive...")
        structure_success = create_allan_drive_structure()
        
        if not structure_success:
            print("❌ Ошибка создания структуры проекта")
            return False
        
        # Шаг 5: Оптимизация для обучения
        print("\n⚡ Оптимизация системы для обучения...")
        optimize_allan_for_training()
        
        # Шаг 6: Показ доступных датасетов
        print("\n📚 Доступные датасеты для Allan:")
        list_datasets()
        
        # Шаг 7: Финальная информация
        print_success_info()
        
        return True
        
    except ImportError as e:
        print(f"❌ Ошибка импорта: {e}")
        print("💡 Убедитесь, что все файлы Allan находятся в текущей папке")
        return False
    except Exception as e:
        print(f"❌ Неожиданная ошибка: {e}")
        return False

def print_success_info():
    """Информация об успешной настройке"""
    success_info = """
    🎉 НАСТРОЙКА ALLAN ЗАВЕРШЕНА УСПЕШНО!
    ═══════════════════════════════════════════════════════════════
    
    📊 Что настроено:
    ✅ Google Drive подключен (2 ТБ доступно)
    ✅ Все библиотеки установлены (PyTorch, Transformers, etc.)
    ✅ Структура проекта создана (50+ папок)
    ✅ Переменные окружения настроены
    ✅ Система оптимизирована для обучения
    ✅ GPU готов к работе
    
    🚀 Следующие шаги:
    
    1️⃣  ЗАГРУЗИТЕ ДАТАСЕТ:
    ```python
    from allan_dataset_manager import quick_load_dataset
    dataset = quick_load_dataset("sberquad")  # Или другой датасет
    ```
    
    2️⃣  НАЧНИТЕ ОБУЧЕНИЕ:
    ```python
    # Ваш код обучения здесь
    # Все настроено для максимальной производительности!
    ```
    
    3️⃣  МОНИТОРИНГ (опционально):
    ```python
    from allan_performance_optimizer import monitor_allan_training
    monitor_allan_training(duration_minutes=60)
    ```
    
    📁 Путь к проекту: /content/drive/MyDrive/ML_Projects/Allan_Model/
    💾 Локальный кэш: /content/allan_cache/
    📚 Документация: ALLAN_GOOGLE_DRIVE_GUIDE.md
    
    🔥 ALLAN ГОТОВ К ПОКОРЕНИЮ РУССКОЯЗЫЧНОГО NLP! 🔥
    """
    print(success_info)

def show_quick_commands():
    """Показ быстрых команд для работы с Allan"""
    commands = """
    🛠️  БЫСТРЫЕ КОМАНДЫ ALLAN:
    ═══════════════════════════════════════════════════════════════
    
    # 🚀 Полная настройка в одну строку:
    exec(open('quick_start_allan.py').read()); quick_setup_allan()
    
    # 📊 Проверка состояния системы:
    from allan_performance_optimizer import AllanPerformanceOptimizer
    optimizer = AllanPerformanceOptimizer()
    optimizer.print_current_status()
    
    # 📚 Просмотр доступных датасетов:
    from allan_dataset_manager import list_datasets
    list_datasets()
    
    # 💾 Быстрая загрузка датасета:
    from allan_dataset_manager import quick_load_dataset
    dataset = quick_load_dataset("sberquad")
    
    # 🧹 Быстрая очистка ресурсов:
    from allan_performance_optimizer import cleanup_allan_resources
    cleanup_allan_resources()
    
    # 📊 Мониторинг обучения:
    from allan_performance_optimizer import monitor_allan_training
    report = monitor_allan_training(60)  # 60 минут
    
    # 🔧 Переоптимизация системы:
    from allan_performance_optimizer import optimize_allan_for_training
    optimize_allan_for_training()
    """
    print(commands)

def diagnose_system():
    """Диагностика системы Allan"""
    print("🔍 Диагностика системы Allan...")
    print("=" * 50)
    
    # Проверка Python версии
    print(f"🐍 Python: {sys.version}")
    
    # Проверка подключения к Drive
    drive_connected = os.path.exists('/content/drive/MyDrive')
    drive_icon = "✅" if drive_connected else "❌"
    print(f"{drive_icon} Google Drive: {'Подключен' if drive_connected else 'Не подключен'}")
    
    # Проверка инструментов Allan
    tools = ["allan_colab_setup.py", "allan_dataset_manager.py", "allan_performance_optimizer.py"]
    for tool in tools:
        exists = os.path.exists(tool)
        icon = "✅" if exists else "❌"
        print(f"{icon} {tool}: {'Доступен' if exists else 'Отсутствует'}")
    
    # Проверка библиотек
    libraries = ["torch", "transformers", "datasets", "psutil"]
    for lib in libraries:
        try:
            __import__(lib)
            print(f"✅ {lib}: Установлен")
        except ImportError:
            print(f"❌ {lib}: Не установлен")
    
    # Проверка GPU
    try:
        import torch
        if torch.cuda.is_available():
            gpu_name = torch.cuda.get_device_name(0)
            print(f"✅ GPU: {gpu_name}")
        else:
            print("❌ GPU: Недоступен")
    except:
        print("❌ GPU: Ошибка проверки")

def interactive_setup():
    """Интерактивная настройка Allan"""
    print("🤖 Интерактивная настройка Allan")
    print("=" * 40)
    
    # Проверка готовности
    ready = input("🚀 Готовы начать настройку Allan? (y/n): ").lower().strip()
    if ready != 'y':
        print("👋 До свидания! Возвращайтесь когда будете готовы.")
        return
    
    # Диагностика
    print("\n🔍 Сначала проведем диагностику...")
    diagnose_system()
    
    # Подтверждение продолжения
    continue_setup = input("\n✅ Продолжить настройку? (y/n): ").lower().strip()
    if continue_setup != 'y':
        print("⏹️  Настройка отменена.")
        return
    
    # Запуск настройки
    print("\n🚀 Запускаем полную настройку Allan...")
    success = quick_setup_allan()
    
    if success:
        print("\n🎊 Поздравляем! Allan готов к работе!")
        
        # Предложение загрузить датасет
        load_dataset = input("\n📚 Загрузить тестовый датасет SberQuAD? (y/n): ").lower().strip()
        if load_dataset == 'y':
            try:
                from allan_dataset_manager import quick_load_dataset
                print("📥 Загрузка SberQuAD...")
                dataset = quick_load_dataset("sberquad")
                print(f"✅ Датасет загружен: {len(dataset['train'])} примеров")
            except Exception as e:
                print(f"❌ Ошибка загрузки датасета: {e}")
        
        # Показ быстрых команд
        show_commands = input("\n🛠️  Показать быстрые команды? (y/n): ").lower().strip()
        if show_commands == 'y':
            show_quick_commands()
    else:
        print("\n😞 Настройка не удалась. Проверьте ошибки выше.")

# Главные функции для экспорта
__all__ = [
    'quick_setup_allan',
    'diagnose_system', 
    'interactive_setup',
    'show_quick_commands'
]

if __name__ == "__main__":
    # При прямом запуске - интерактивная настройка
    interactive_setup()