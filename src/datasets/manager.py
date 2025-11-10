#!/usr/bin/env python3
"""
Allan Dataset Manager - Умная система управления датасетами
Автоматический выбор оптимальной стратегии загрузки датасетов с Google Drive
"""

import os
import time
import shutil
import zipfile
import tarfile
from pathlib import Path
from typing import Dict, List, Optional, Tuple, Union
from dataclasses import dataclass
import psutil
import subprocess

@dataclass
class DatasetInfo:
    """Информация о датасете"""
    name: str
    size_mb: float
    format: str  # 'zip', 'tar', 'directory', 'hf_dataset'
    path: str
    description: str = ""
    language: str = "ru"
    task_type: str = "general"

class AllanDatasetManager:
    """Умный менеджер датасетов для проекта Allan"""
    
    def __init__(self, project_path: str = "/content/drive/MyDrive/ML_Projects/Allan_Model"):
        self.project_path = project_path
        self.datasets_path = f"{project_path}/datasets"
        self.raw_path = f"{self.datasets_path}/raw"
        self.processed_path = f"{self.datasets_path}/processed"
        self.cached_path = f"{self.datasets_path}/cached"
        self.local_cache = "/content/allan_cache"
        
        # Пороговые значения для стратегий загрузки (в МБ)
        self.SMALL_DATASET_THRESHOLD = 100   # < 100 МБ - читать напрямую с Drive
        self.MEDIUM_DATASET_THRESHOLD = 500  # 100-500 МБ - копировать локально
        self.LARGE_DATASET_THRESHOLD = 2000  # > 2 ГБ - потоковая загрузка
        
        # Информация о рекомендованных датасетах для Allan
        self.recommended_datasets = {
            "sberquad": DatasetInfo(
                name="sberquad",
                size_mb=150,
                format="hf_dataset", 
                path="sberbank-ai/sberquad",
                description="Русский датасет вопрос-ответ на основе SQuAD",
                task_type="qa"
            ),
            "rucola": DatasetInfo(
                name="rucola",
                size_mb=50,
                format="hf_dataset",
                path="RussianNLP/rucola", 
                description="Корпус лингвистической приемлемости для русского",
                task_type="classification"
            ),
            "russian_superglue": DatasetInfo(
                name="russian_superglue",
                size_mb=200,
                format="hf_dataset",
                path="russian-nlp/russian-superglue",
                description="Набор задач для оценки русскоязычных моделей",
                task_type="multi_task"
            ),
            "lenta_news": DatasetInfo(
                name="lenta_news", 
                size_mb=2000,
                format="hf_dataset",
                path="IlyaGusev/gazeta",
                description="Новостные статьи Lenta.ru",
                task_type="text_generation"
            ),
            "russian_poems": DatasetInfo(
                name="russian_poems",
                size_mb=150,
                format="hf_dataset", 
                path="IlyaGusev/russian_poems",
                description="Корпус русской поэзии",
                task_type="text_generation"
            )
        }
    
    def get_file_size_mb(self, file_path: str) -> float:
        """Получить размер файла в МБ"""
        try:
            if os.path.isfile(file_path):
                size_bytes = os.path.getsize(file_path)
                return size_bytes / (1024 * 1024)
            elif os.path.isdir(file_path):
                total_size = 0
                for dirpath, dirnames, filenames in os.walk(file_path):
                    for filename in filenames:
                        filepath = os.path.join(dirpath, filename)
                        try:
                            total_size += os.path.getsize(filepath)
                        except (OSError, IOError):
                            continue
                return total_size / (1024 * 1024)
            else:
                return 0.0
        except Exception:
            return 0.0
    
    def get_available_space_gb(self, path: str = "/content") -> float:
        """Получить доступное место на диске в ГБ"""
        try:
            statvfs = os.statvfs(path)
            free_bytes = statvfs.f_frsize * statvfs.f_bavail
            return free_bytes / (1024**3)
        except:
            return 0.0
    
    def choose_loading_strategy(self, dataset_size_mb: float) -> str:
        """Выбор оптимальной стратегии загрузки"""
        available_space_gb = self.get_available_space_gb()
        dataset_size_gb = dataset_size_mb / 1024
        
        print(f"📊 Размер датасета: {dataset_size_mb:.1f} МБ ({dataset_size_gb:.2f} ГБ)")
        print(f"💾 Доступно места: {available_space_gb:.1f} ГБ")
        
        # Проверка достаточности места
        if dataset_size_gb > available_space_gb * 0.8:  # Оставляем 20% запаса
            print("⚠️  Недостаточно места для локального копирования")
            return "streaming"
        
        # Выбор стратегии по размеру
        if dataset_size_mb < self.SMALL_DATASET_THRESHOLD:
            strategy = "direct"
            print(f"🔄 Стратегия: Прямое чтение с Drive (< {self.SMALL_DATASET_THRESHOLD} МБ)")
        elif dataset_size_mb < self.MEDIUM_DATASET_THRESHOLD:
            strategy = "copy_local"
            print(f"🔄 Стратегия: Копирование в локальный кэш (< {self.MEDIUM_DATASET_THRESHOLD} МБ)")
        elif dataset_size_mb < self.LARGE_DATASET_THRESHOLD:
            strategy = "copy_local"
            print(f"🔄 Стратегия: Копирование в локальный кэш (< {self.LARGE_DATASET_THRESHOLD} МБ)")
        else:
            strategy = "streaming"
            print(f"🔄 Стратегия: Потоковая загрузка (> {self.LARGE_DATASET_THRESHOLD} МБ)")
        
        return strategy
    
    def extract_archive(self, archive_path: str, extract_to: str) -> bool:
        """Извлечение архива"""
        try:
            print(f"📦 Извлечение архива: {os.path.basename(archive_path)}")
            
            if archive_path.endswith('.zip'):
                with zipfile.ZipFile(archive_path, 'r') as zip_ref:
                    zip_ref.extractall(extract_to)
            elif archive_path.endswith(('.tar', '.tar.gz', '.tgz')):
                with tarfile.open(archive_path, 'r:*') as tar_ref:
                    tar_ref.extractall(extract_to)
            else:
                print(f"❌ Неподдерживаемый формат архива: {archive_path}")
                return False
            
            print("✅ Архив успешно извлечен")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка извлечения архива: {e}")
            return False
    
    def copy_with_progress(self, src: str, dst: str) -> bool:
        """Копирование файла с отображением прогресса"""
        try:
            file_size = os.path.getsize(src)
            print(f"📁 Копирование: {os.path.basename(src)} ({file_size / (1024*1024):.1f} МБ)")
            
            # Для больших файлов показываем прогресс
            if file_size > 50 * 1024 * 1024:  # > 50 МБ
                with open(src, 'rb') as fsrc:
                    with open(dst, 'wb') as fdst:
                        copied = 0
                        chunk_size = 1024 * 1024  # 1 МБ чанки
                        
                        while True:
                            chunk = fsrc.read(chunk_size)
                            if not chunk:
                                break
                            fdst.write(chunk)
                            copied += len(chunk)
                            
                            # Показываем прогресс каждые 10%
                            progress = (copied / file_size) * 100
                            if copied % (file_size // 10 + 1) == 0:
                                print(f"  📈 {progress:.0f}%")
            else:
                shutil.copy2(src, dst)
            
            print("✅ Копирование завершено")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка копирования: {e}")
            return False
    
    def load_dataset_direct(self, dataset_path: str, **kwargs):
        """Прямая загрузка датасета с Google Drive"""
        print("🔗 Прямая загрузка с Google Drive...")
        
        try:
            from datasets import load_dataset, load_from_disk
            
            if os.path.isdir(dataset_path):
                # Загрузка сохраненного датасета
                dataset = load_from_disk(dataset_path)
            else:
                # Загрузка по имени/пути
                dataset = load_dataset(dataset_path, **kwargs)
            
            print("✅ Датасет загружен напрямую с Drive")
            return dataset
            
        except Exception as e:
            print(f"❌ Ошибка прямой загрузки: {e}")
            return None
    
    def load_dataset_local_copy(self, dataset_path: str, dataset_name: str, **kwargs):
        """Загрузка датасета через локальное копирование"""
        print("💾 Загрузка через локальное копирование...")
        
        try:
            # Создание локального кэша
            local_dataset_path = f"{self.local_cache}/{dataset_name}"
            os.makedirs(local_dataset_path, exist_ok=True)
            
            # Копирование данных
            if os.path.isfile(dataset_path):
                # Копирование файла
                local_file = f"{local_dataset_path}/{os.path.basename(dataset_path)}"
                if not self.copy_with_progress(dataset_path, local_file):
                    return None
                
                # Извлечение если это архив
                if dataset_path.endswith(('.zip', '.tar', '.tar.gz', '.tgz')):
                    if not self.extract_archive(local_file, local_dataset_path):
                        return None
                    dataset_path = local_dataset_path
                else:
                    dataset_path = local_file
                    
            elif os.path.isdir(dataset_path):
                # Копирование директории
                print(f"📁 Копирование директории...")
                shutil.copytree(dataset_path, local_dataset_path, dirs_exist_ok=True)
                dataset_path = local_dataset_path
            
            # Загрузка датасета из локального кэша
            from datasets import load_dataset, load_from_disk
            
            if os.path.isdir(dataset_path) and any(f.endswith('.arrow') for f in os.listdir(dataset_path)):
                dataset = load_from_disk(dataset_path)
            else:
                dataset = load_dataset(dataset_path, **kwargs)
            
            print("✅ Датасет загружен из локального кэша")
            return dataset
            
        except Exception as e:
            print(f"❌ Ошибка локального копирования: {e}")
            return None
    
    def load_dataset_streaming(self, dataset_path: str, **kwargs):
        """Потоковая загрузка датасета"""
        print("🌊 Потоковая загрузка датасета...")
        
        try:
            from datasets import load_dataset
            
            # Включаем потоковый режим
            kwargs['streaming'] = True
            dataset = load_dataset(dataset_path, **kwargs)
            
            print("✅ Датасет загружен в потоковом режиме")
            return dataset
            
        except Exception as e:
            print(f"❌ Ошибка потоковой загрузки: {e}")
            return None
    
    def load_huggingface_dataset(self, dataset_name: str, **kwargs):
        """Загрузка датасета из Hugging Face Hub"""
        print(f"🤗 Загрузка датасета из Hugging Face: {dataset_name}")
        
        # Проверяем, есть ли информация о датасете
        if dataset_name in self.recommended_datasets:
            dataset_info = self.recommended_datasets[dataset_name]
            dataset_path = dataset_info.path
            size_mb = dataset_info.size_mb
            
            print(f"📝 {dataset_info.description}")
            print(f"🎯 Тип задачи: {dataset_info.task_type}")
        else:
            dataset_path = dataset_name
            size_mb = 100  # Предполагаемый размер по умолчанию
        
        # Выбираем стратегию загрузки
        strategy = self.choose_loading_strategy(size_mb)
        
        try:
            from datasets import load_dataset
            
            if strategy == "streaming":
                kwargs['streaming'] = True
                dataset = load_dataset(dataset_path, **kwargs)
            else:
                dataset = load_dataset(dataset_path, **kwargs)
                
                # Сохраняем в кэш на Drive для будущего использования
                if strategy == "copy_local":
                    cache_path = f"{self.cached_path}/{dataset_name}"
                    os.makedirs(cache_path, exist_ok=True)
                    dataset.save_to_disk(cache_path)
                    print(f"💾 Датасет сохранен в кэш: {cache_path}")
            
            print("✅ Датасет успешно загружен")
            return dataset
            
        except Exception as e:
            print(f"❌ Ошибка загрузки датасета: {e}")
            return None
    
    def load_dataset(self, dataset_source: str, **kwargs):
        """Универсальная функция загрузки датасета"""
        print(f"🚀 Загрузка датасета: {dataset_source}")
        print("=" * 50)
        
        # Определяем тип источника
        if dataset_source in self.recommended_datasets:
            # Рекомендованный датасет
            return self.load_huggingface_dataset(dataset_source, **kwargs)
        
        elif os.path.exists(dataset_source):
            # Локальный файл/папка
            size_mb = self.get_file_size_mb(dataset_source)
            strategy = self.choose_loading_strategy(size_mb)
            
            if strategy == "direct":
                return self.load_dataset_direct(dataset_source, **kwargs)
            elif strategy == "copy_local":
                dataset_name = os.path.basename(dataset_source)
                return self.load_dataset_local_copy(dataset_source, dataset_name, **kwargs)
            else:  # streaming
                return self.load_dataset_streaming(dataset_source, **kwargs)
        
        else:
            # Предполагаем, что это путь Hugging Face
            return self.load_huggingface_dataset(dataset_source, **kwargs)
    
    def list_available_datasets(self):
        """Список доступных рекомендованных датасетов"""
        print("📚 Рекомендованные датасеты для Allan:")
        print("=" * 60)
        
        for name, info in self.recommended_datasets.items():
            print(f"\n🔹 {name}")
            print(f"  📊 Размер: {info.size_mb} МБ")
            print(f"  🎯 Задача: {info.task_type}")
            print(f"  📝 {info.description}")
            print(f"  🔗 Путь: {info.path}")
    
    def get_dataset_stats(self, dataset, dataset_name: str = "dataset"):
        """Получение статистики датасета"""
        print(f"📊 Статистика датасета '{dataset_name}':")
        
        try:
            if hasattr(dataset, 'num_rows'):
                print(f"  📈 Количество примеров: {dataset.num_rows:,}")
            
            if hasattr(dataset, 'features'):
                print(f"  🏷️  Признаки: {list(dataset.features.keys())}")
            
            if hasattr(dataset, 'column_names'):
                print(f"  📋 Столбцы: {dataset.column_names}")
            
            # Пример данных
            if hasattr(dataset, '__iter__'):
                print("  📝 Пример данных:")
                try:
                    example = next(iter(dataset))
                    for key, value in list(example.items())[:3]:  # Первые 3 поля
                        value_str = str(value)[:100] + "..." if len(str(value)) > 100 else str(value)
                        print(f"    {key}: {value_str}")
                except:
                    pass
                    
        except Exception as e:
            print(f"  ⚠️  Ошибка получения статистики: {e}")
    
    def clear_local_cache(self):
        """Очистка локального кэша"""
        print("🧹 Очистка локального кэша...")
        
        try:
            if os.path.exists(self.local_cache):
                shutil.rmtree(self.local_cache)
                print("✅ Локальный кэш очищен")
            
            os.makedirs(self.local_cache, exist_ok=True)
            
        except Exception as e:
            print(f"❌ Ошибка очистки кэша: {e}")
    
    def monitor_resources(self):
        """Мониторинг использования ресурсов"""
        print("🔍 Мониторинг ресурсов:")
        
        try:
            # RAM
            memory = psutil.virtual_memory()
            print(f"  🧠 RAM: {memory.percent:.1f}% использовано ({memory.used / (1024**3):.1f}/{memory.total / (1024**3):.1f} ГБ)")
            
            # Диск
            disk = psutil.disk_usage("/content")
            disk_percent = (disk.used / disk.total) * 100
            print(f"  💾 Диск: {disk_percent:.1f}% использовано ({disk.used / (1024**3):.1f}/{disk.total / (1024**3):.1f} ГБ)")
            
            # GPU (если доступен)
            try:
                import torch
                if torch.cuda.is_available():
                    gpu_memory = torch.cuda.get_device_properties(0).total_memory
                    gpu_allocated = torch.cuda.memory_allocated(0)
                    gpu_percent = (gpu_allocated / gpu_memory) * 100
                    print(f"  🎮 GPU: {gpu_percent:.1f}% использовано ({gpu_allocated / (1024**3):.1f}/{gpu_memory / (1024**3):.1f} ГБ)")
            except:
                pass
                
        except Exception as e:
            print(f"❌ Ошибка мониторинга: {e}")


def quick_load_dataset(dataset_name: str, **kwargs):
    """Быстрая функция для загрузки датасета"""
    manager = AllanDatasetManager()
    return manager.load_dataset(dataset_name, **kwargs)


def list_datasets():
    """Быстрая функция для просмотра доступных датасетов"""
    manager = AllanDatasetManager()
    manager.list_available_datasets()


if __name__ == "__main__":
    # Демонстрация работы менеджера датасетов
    manager = AllanDatasetManager()
    
    print("🔥 Allan Dataset Manager - Демонстрация")
    print("=" * 50)
    
    # Показываем доступные датасеты
    manager.list_available_datasets()
    
    # Мониторинг ресурсов
    print("\n")
    manager.monitor_resources()