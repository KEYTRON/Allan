#!/usr/bin/env python3
"""
Allan Model - Google Colab Setup Script
Автоматическая настройка среды Colab для обучения модели Allan
"""

import os
import sys
import time
import shutil
import subprocess
from pathlib import Path
from typing import Optional, Dict, List

class AllanColabSetup:
    """Класс для автоматической настройки Colab среды для проекта Allan"""
    
    def __init__(self):
        self.drive_path = "/content/drive"
        self.project_path = "/content/drive/MyDrive/ML_Projects/Allan_Model"
        self.local_cache = "/content/allan_cache"
        
    def mount_drive(self) -> bool:
        """Подключение Google Drive"""
        try:
            print("🔗 Подключение Google Drive...")
            from google.colab import drive
            drive.mount(self.drive_path, force_remount=True)
            
            if os.path.exists(self.drive_path):
                print("✅ Google Drive успешно подключен!")
                return True
            else:
                print("❌ Ошибка подключения Google Drive")
                return False
                
        except Exception as e:
            print(f"❌ Ошибка при подключении Drive: {e}")
            return False
    
    def install_dependencies(self) -> bool:
        """Установка всех необходимых библиотек"""
        print("📦 Установка библиотек для Allan...")
        
        packages = [
            "transformers[torch]>=4.35.0",
            "datasets>=2.14.0", 
            "tokenizers>=0.14.0",
            "torch>=2.0.0",
            "accelerate>=0.24.0",
            "evaluate>=0.4.0",
            "scikit-learn>=1.3.0",
            "pymorphy2[fast]>=0.9.1",  # Для русского языка
            "razdel>=0.5.0",           # Токенизация русского
            "sentencepiece>=0.1.99",   # Для некоторых моделей
            "wandb>=0.16.0",           # Для логирования
            "tensorboard>=2.14.0",     # Альтернативное логирование
            "matplotlib>=3.7.0",
            "seaborn>=0.12.0",
            "tqdm>=4.65.0",
            "psutil>=5.9.0",           # Мониторинг ресурсов
            "gpustat>=1.1.0",          # Мониторинг GPU
        ]
        
        try:
            for package in packages:
                print(f"  📥 Установка {package}...")
                subprocess.run([
                    sys.executable, "-m", "pip", "install", "-q", package
                ], check=True, capture_output=True)
            
            print("✅ Все библиотеки установлены!")
            return True
            
        except subprocess.CalledProcessError as e:
            print(f"❌ Ошибка установки пакетов: {e}")
            return False
    
    def create_project_structure(self) -> bool:
        """Создание структуры папок проекта на Google Drive"""
        print("📁 Создание структуры проекта...")
        
        directories = [
            self.project_path,
            f"{self.project_path}/datasets",
            f"{self.project_path}/datasets/raw",
            f"{self.project_path}/datasets/processed", 
            f"{self.project_path}/datasets/cached",
            f"{self.project_path}/models",
            f"{self.project_path}/models/checkpoints",
            f"{self.project_path}/models/final",
            f"{self.project_path}/models/tokenizers",
            f"{self.project_path}/logs",
            f"{self.project_path}/logs/tensorboard",
            f"{self.project_path}/logs/wandb",
            f"{self.project_path}/configs",
            f"{self.project_path}/scripts",
            f"{self.project_path}/notebooks",
            f"{self.project_path}/results",
            f"{self.project_path}/cache",
        ]
        
        try:
            for directory in directories:
                os.makedirs(directory, exist_ok=True)
                print(f"  📂 {directory}")
            
            # Создание локального кэша
            os.makedirs(self.local_cache, exist_ok=True)
            
            print("✅ Структура проекта создана!")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка создания структуры: {e}")
            return False
    
    def setup_environment(self) -> bool:
        """Настройка переменных окружения"""
        print("🔧 Настройка окружения...")
        
        try:
            # Переменные для Hugging Face
            os.environ["HF_HOME"] = f"{self.project_path}/cache/huggingface"
            os.environ["TRANSFORMERS_CACHE"] = f"{self.project_path}/cache/transformers"
            os.environ["HF_DATASETS_CACHE"] = f"{self.project_path}/cache/datasets"
            
            # Переменные для PyTorch
            os.environ["TORCH_HOME"] = f"{self.project_path}/cache/torch"
            
            # Переменные для проекта Allan
            os.environ["ALLAN_PROJECT_PATH"] = self.project_path
            os.environ["ALLAN_CACHE_PATH"] = self.local_cache
            
            # Создание кэш-папок
            cache_dirs = [
                f"{self.project_path}/cache/huggingface",
                f"{self.project_path}/cache/transformers", 
                f"{self.project_path}/cache/datasets",
                f"{self.project_path}/cache/torch",
            ]
            
            for cache_dir in cache_dirs:
                os.makedirs(cache_dir, exist_ok=True)
            
            print("✅ Окружение настроено!")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка настройки окружения: {e}")
            return False
    
    def verify_setup(self) -> Dict[str, bool]:
        """Проверка корректности настройки"""
        print("🔍 Проверка установки...")
        
        results = {}
        
        # Проверка подключения Drive
        results["drive_mounted"] = os.path.exists(self.drive_path)
        
        # Проверка структуры проекта
        results["project_structure"] = os.path.exists(self.project_path)
        
        # Проверка библиотек
        try:
            import torch
            import transformers
            import datasets
            results["libraries"] = True
        except ImportError:
            results["libraries"] = False
        
        # Проверка GPU
        try:
            import torch
            results["gpu_available"] = torch.cuda.is_available()
            if results["gpu_available"]:
                gpu_name = torch.cuda.get_device_name(0)
                print(f"  🎮 GPU: {gpu_name}")
        except:
            results["gpu_available"] = False
        
        # Отчет
        print("\n📊 Результаты проверки:")
        for check, status in results.items():
            status_icon = "✅" if status else "❌"
            print(f"  {status_icon} {check}: {status}")
        
        return results
    
    def get_system_info(self) -> Dict:
        """Получение информации о системе"""
        print("💻 Информация о системе:")
        
        info = {}
        
        try:
            import psutil
            import torch
            
            # CPU информация
            info["cpu_count"] = psutil.cpu_count()
            info["cpu_percent"] = psutil.cpu_percent(interval=1)
            
            # RAM информация  
            memory = psutil.virtual_memory()
            info["ram_total_gb"] = round(memory.total / (1024**3), 2)
            info["ram_available_gb"] = round(memory.available / (1024**3), 2)
            info["ram_used_percent"] = memory.percent
            
            # Диск информация
            disk = psutil.disk_usage("/content")
            info["disk_total_gb"] = round(disk.total / (1024**3), 2)
            info["disk_free_gb"] = round(disk.free / (1024**3), 2)
            info["disk_used_percent"] = round((disk.used / disk.total) * 100, 2)
            
            # GPU информация
            if torch.cuda.is_available():
                info["gpu_name"] = torch.cuda.get_device_name(0)
                info["gpu_memory_gb"] = round(torch.cuda.get_device_properties(0).total_memory / (1024**3), 2)
            
            # Вывод информации
            print(f"  🖥️  CPU: {info['cpu_count']} cores ({info['cpu_percent']}% загрузка)")
            print(f"  🧠 RAM: {info['ram_available_gb']}/{info['ram_total_gb']} GB доступно ({info['ram_used_percent']}% использовано)")
            print(f"  💾 Диск: {info['disk_free_gb']}/{info['disk_total_gb']} GB свободно ({info['disk_used_percent']}% использовано)")
            
            if "gpu_name" in info:
                print(f"  🎮 GPU: {info['gpu_name']} ({info['gpu_memory_gb']} GB)")
            else:
                print("  ⚠️  GPU недоступен")
                
        except Exception as e:
            print(f"❌ Ошибка получения системной информации: {e}")
            
        return info
    
    def setup_allan_colab(self) -> bool:
        """Полная настройка среды для Allan"""
        print("🚀 Запуск настройки Allan Colab Environment...")
        print("=" * 50)
        
        steps = [
            ("Подключение Google Drive", self.mount_drive),
            ("Установка библиотек", self.install_dependencies), 
            ("Создание структуры проекта", self.create_project_structure),
            ("Настройка окружения", self.setup_environment),
        ]
        
        success_count = 0
        
        for step_name, step_function in steps:
            print(f"\n🔄 {step_name}...")
            if step_function():
                success_count += 1
            else:
                print(f"❌ Ошибка на этапе: {step_name}")
        
        print(f"\n{'='*50}")
        
        if success_count == len(steps):
            print("🎉 Настройка завершена успешно!")
            
            # Проверка установки
            self.verify_setup()
            
            # Информация о системе
            self.get_system_info()
            
            print(f"\n📁 Путь к проекту: {self.project_path}")
            print(f"💾 Локальный кэш: {self.local_cache}")
            print("\n🔥 Allan готов к обучению!")
            
            return True
        else:
            print(f"⚠️  Настройка завершена с ошибками ({success_count}/{len(steps)} успешно)")
            return False


def setup_allan():
    """Главная функция для настройки Allan в Colab"""
    setup = AllanColabSetup()
    return setup.setup_allan_colab()


if __name__ == "__main__":
    # Запуск настройки при прямом вызове скрипта
    setup_allan()