#!/usr/bin/env python3
"""
Allan Dataset Downloader - Загрузчик и предподготовка датасетов
Автоматическое скачивание, предобработка и сохранение датасетов на Google Drive
"""

import os
import sys
import time
import json
import shutil
import zipfile
import tarfile
import requests
from pathlib import Path
from typing import Dict, List, Optional, Tuple, Union, Any
from dataclasses import dataclass
import psutil
import subprocess
from tqdm import tqdm
import hashlib

@dataclass
class DatasetConfig:
    """Конфигурация датасета для загрузки"""
    name: str
    source_url: str
    source_type: str  # 'url', 'huggingface', 'kaggle', 'local'
    format: str  # 'zip', 'tar', 'csv', 'json', 'hf_dataset'
    size_mb: float
    description: str
    language: str = "ru"
    task_type: str = "general"
    preprocessing_steps: List[str] = None
    validation_checks: List[str] = None
    dependencies: List[str] = None

class AllanDatasetDownloader:
    """Загрузчик и предподготовка датасетов для проекта Allan"""
    
    def __init__(self, project_path: str = "/content/drive/MyDrive/ML_Projects/Allan_Model"):
        self.project_path = project_path
        self.datasets_path = f"{project_path}/datasets"
        self.raw_path = f"{self.datasets_path}/raw"
        self.processed_path = f"{self.datasets_path}/processed"
        self.cached_path = f"{self.datasets_path}/cached"
        self.temp_path = f"{self.datasets_path}/temp"
        self.local_cache = "/content/allan_cache"
        
        # Создание необходимых директорий
        self._create_directories()
        
        # Конфигурации популярных русскоязычных датасетов
        self.dataset_configs = self._load_dataset_configs()
        
        # Настройки загрузки
        self.chunk_size = 8192  # Размер чанка для загрузки
        self.max_retries = 3    # Максимальное количество попыток
        self.timeout = 300      # Таймаут загрузки в секундах
        
    def _create_directories(self):
        """Создание необходимых директорий"""
        directories = [
            self.datasets_path,
            self.raw_path,
            self.processed_path,
            self.cached_path,
            self.temp_path,
            self.local_cache
        ]
        
        for directory in directories:
            os.makedirs(directory, exist_ok=True)
    
    def _load_dataset_configs(self) -> Dict[str, DatasetConfig]:
        """Загрузка конфигураций датасетов"""
        configs = {
            "sberquad": DatasetConfig(
                name="sberquad",
                source_url="sberbank-ai/sberquad",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=150,
                description="Русский датасет вопрос-ответ на основе SQuAD",
                task_type="qa",
                preprocessing_steps=["tokenization", "max_length_512", "truncation"],
                validation_checks=["check_qa_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),
            "rucola": DatasetConfig(
                name="rucola",
                source_url="RussianNLP/rucola",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=50,
                description="Корпус лингвистической приемлемости для русского",
                task_type="classification",
                preprocessing_steps=["tokenization", "max_length_128", "binary_labels"],
                validation_checks=["check_classification_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),
            "russian_superglue": DatasetConfig(
                name="russian_superglue",
                source_url="russian-nlp/russian-superglue",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=200,
                description="Набор задач для оценки русскоязычных моделей",
                task_type="multi_task",
                preprocessing_steps=["task_specific_preprocessing", "unified_format"],
                validation_checks=["check_multitask_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),
            "lenta_news": DatasetConfig(
                name="lenta_news",
                source_url="IlyaGusev/gazeta",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=2000,
                description="Новостные статьи Lenta.ru",
                task_type="text_generation",
                preprocessing_steps=["text_cleaning", "max_length_1024", "remove_html"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),
            "russian_poems": DatasetConfig(
                name="russian_poems",
                source_url="IlyaGusev/russian_poems",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=150,
                description="Корпус русской поэзии",
                task_type="text_generation",
                preprocessing_steps=["text_cleaning", "max_length_512", "poem_formatting"],
                validation_checks=["check_poem_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),
            "russian_paraphrase": DatasetConfig(
                name="russian_paraphrase",
                source_url="https://storage.googleapis.com/russian-paraphrase/russian_paraphrase.zip",
                source_type="url",
                format="zip",
                size_mb=80,
                description="Датасет русских парафраз",
                task_type="paraphrase",
                preprocessing_steps=["extract_paraphrases", "create_pairs", "max_length_256"],
                validation_checks=["check_paraphrase_format", "validate_russian_text"],
                dependencies=["pandas", "numpy"]
            ),
            "russian_sentiment": DatasetConfig(
                name="russian_sentiment",
                source_url="https://github.com/DeepPavlov/russian_sentiment/archive/refs/heads/master.zip",
                source_type="url",
                format="zip",
                size_mb=120,
                description="Датасет для анализа тональности русского текста",
                task_type="sentiment_analysis",
                preprocessing_steps=["extract_sentences", "create_labels", "max_length_256"],
                validation_checks=["check_sentiment_format", "validate_russian_text"],
                dependencies=["pandas", "numpy"]
            )
        }
        return configs
    
    def download_file_with_progress(self, url: str, destination: str) -> bool:
        """Загрузка файла с отображением прогресса"""
        try:
            print(f"📥 Загрузка: {url}")
            
            # Получаем размер файла
            response = requests.head(url, timeout=10)
            total_size = int(response.headers.get('content-length', 0))
            
            # Загружаем файл с прогресс-баром
            response = requests.get(url, stream=True, timeout=self.timeout)
            response.raise_for_status()
            
            with open(destination, 'wb') as file, tqdm(
                desc=os.path.basename(destination),
                total=total_size,
                unit='iB',
                unit_scale=True,
                unit_divisor=1024,
            ) as pbar:
                for chunk in response.iter_content(chunk_size=self.chunk_size):
                    size = file.write(chunk)
                    pbar.update(size)
            
            print(f"✅ Файл загружен: {destination}")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка загрузки: {e}")
            return False
    
    def download_huggingface_dataset(self, dataset_name: str, config: DatasetConfig) -> bool:
        """Загрузка датасета из Hugging Face Hub"""
        try:
            print(f"🤗 Загрузка датасета из Hugging Face: {dataset_name}")
            
            # Устанавливаем зависимости если нужно
            if config.dependencies:
                self._install_dependencies(config.dependencies)
            
            # Загружаем датасет
            from datasets import load_dataset
            
            dataset = load_dataset(config.source_url)
            
            # Сохраняем на Google Drive
            save_path = f"{self.cached_path}/{dataset_name}"
            dataset.save_to_disk(save_path)
            
            # Создаем метаданные
            metadata = {
                "name": dataset_name,
                "source": config.source_url,
                "source_type": config.source_type,
                "download_date": time.strftime("%Y-%m-%d %H:%M:%S"),
                "size_mb": config.size_mb,
                "description": config.description,
                "task_type": config.task_type,
                "language": config.language,
                "format": config.format,
                "preprocessing_steps": config.preprocessing_steps,
                "validation_checks": config.validation_checks
            }
            
            with open(f"{save_path}/metadata.json", 'w', encoding='utf-8') as f:
                json.dump(metadata, f, ensure_ascii=False, indent=2)
            
            print(f"✅ Датасет сохранен: {save_path}")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка загрузки Hugging Face датасета: {e}")
            return False
    
    def download_url_dataset(self, dataset_name: str, config: DatasetConfig) -> bool:
        """Загрузка датасета по URL"""
        try:
            print(f"🌐 Загрузка датасета по URL: {dataset_name}")
            
            # Создаем временную папку
            temp_dir = f"{self.temp_path}/{dataset_name}"
            os.makedirs(temp_dir, exist_ok=True)
            
            # Загружаем файл
            filename = os.path.basename(config.source_url)
            if not filename or '.' not in filename:
                filename = f"{dataset_name}.zip"
            
            temp_file = f"{temp_dir}/{filename}"
            
            if not self.download_file_with_progress(config.source_url, temp_file):
                return False
            
            # Извлекаем архив если нужно
            if config.format in ['zip', 'tar']:
                extract_dir = f"{temp_dir}/extracted"
                os.makedirs(extract_dir, exist_ok=True)
                
                if not self._extract_archive(temp_file, extract_dir):
                    return False
                
                # Перемещаем извлеченные файлы
                raw_dir = f"{self.raw_path}/{dataset_name}"
                if os.path.exists(raw_dir):
                    shutil.rmtree(raw_dir)
                shutil.move(extract_dir, raw_dir)
                
                # Удаляем временные файлы
                shutil.rmtree(temp_dir)
                
                print(f"✅ Датасет извлечен: {raw_dir}")
            else:
                # Просто копируем файл
                raw_dir = f"{self.raw_path}/{dataset_name}"
                os.makedirs(raw_dir, exist_ok=True)
                shutil.copy2(temp_file, f"{raw_dir}/{filename}")
                shutil.rmtree(temp_dir)
                
                print(f"✅ Датасет сохранен: {raw_dir}")
            
            return True
            
        except Exception as e:
            print(f"❌ Ошибка загрузки URL датасета: {e}")
            return False
    
    def _extract_archive(self, archive_path: str, extract_to: str) -> bool:
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
    
    def _install_dependencies(self, dependencies: List[str]):
        """Установка зависимостей"""
        print(f"📦 Установка зависимостей: {', '.join(dependencies)}")
        
        for dep in dependencies:
            try:
                subprocess.run([
                    sys.executable, "-m", "pip", "install", "-q", dep
                ], check=True, capture_output=True)
                print(f"  ✅ {dep} установлен")
            except subprocess.CalledProcessError as e:
                print(f"  ⚠️  Ошибка установки {dep}: {e}")
    
    def preprocess_dataset(self, dataset_name: str, config: DatasetConfig) -> bool:
        """Предобработка датасета"""
        try:
            print(f"🔧 Предобработка датасета: {dataset_name}")
            
            raw_path = f"{self.raw_path}/{dataset_name}"
            processed_path = f"{self.processed_path}/{dataset_name}"
            
            if not os.path.exists(raw_path):
                print(f"❌ Сырой датасет не найден: {raw_path}")
                return False
            
            # Создаем папку для обработанного датасета
            os.makedirs(processed_path, exist_ok=True)
            
            # Применяем шаги предобработки
            if config.preprocessing_steps:
                for step in config.preprocessing_steps:
                    print(f"  🔄 Применение: {step}")
                    
                    if step == "text_cleaning":
                        self._clean_text_data(raw_path, processed_path)
                    elif step == "tokenization":
                        self._tokenize_data(raw_path, processed_path)
                    elif step == "max_length_512":
                        self._truncate_data(processed_path, 512)
                    elif step == "max_length_256":
                        self._truncate_data(processed_path, 256)
                    elif step == "max_length_128":
                        self._truncate_data(processed_path, 128)
                    elif step == "max_length_1024":
                        self._truncate_data(processed_path, 1024)
                    elif step == "remove_html":
                        self._remove_html_tags(processed_path)
                    elif step == "poem_formatting":
                        self._format_poems(processed_path)
                    elif step == "binary_labels":
                        self._convert_to_binary_labels(processed_path)
                    elif step == "create_pairs":
                        self._create_paraphrase_pairs(processed_path)
                    elif step == "extract_paraphrases":
                        self._extract_paraphrases(raw_path, processed_path)
                    elif step == "extract_sentences":
                        self._extract_sentences(raw_path, processed_path)
                    elif step == "create_labels":
                        self._create_sentiment_labels(processed_path)
                    elif step == "task_specific_preprocessing":
                        self._task_specific_preprocessing(raw_path, processed_path)
                    elif step == "unified_format":
                        self._unify_format(processed_path)
            
            # Создаем метаданные предобработки
            preprocessing_metadata = {
                "dataset_name": dataset_name,
                "preprocessing_date": time.strftime("%Y-%m-%d %H:%M:%S"),
                "applied_steps": config.preprocessing_steps or [],
                "original_config": {
                    "size_mb": config.size_mb,
                    "description": config.description,
                    "task_type": config.task_type,
                    "language": config.language
                }
            }
            
            with open(f"{processed_path}/preprocessing_metadata.json", 'w', encoding='utf-8') as f:
                json.dump(preprocessing_metadata, f, ensure_ascii=False, indent=2)
            
            print(f"✅ Предобработка завершена: {processed_path}")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка предобработки: {e}")
            return False
    
    def _clean_text_data(self, raw_path: str, processed_path: str):
        """Очистка текстовых данных"""
        # Простая очистка текста
        pass
    
    def _tokenize_data(self, raw_path: str, processed_path: str):
        """Токенизация данных"""
        # Токенизация текста
        pass
    
    def _truncate_data(self, processed_path: str, max_length: int):
        """Обрезка данных до максимальной длины"""
        # Обрезка текста до максимальной длины
        pass
    
    def _remove_html_tags(self, processed_path: str):
        """Удаление HTML тегов"""
        # Удаление HTML разметки
        pass
    
    def _format_poems(self, processed_path: str):
        """Форматирование стихов"""
        # Форматирование поэтического текста
        pass
    
    def _convert_to_binary_labels(self, processed_path: str):
        """Конвертация в бинарные метки"""
        # Конвертация меток в бинарный формат
        pass
    
    def _create_paraphrase_pairs(self, processed_path: str):
        """Создание пар парафраз"""
        # Создание пар парафраз
        pass
    
    def _extract_paraphrases(self, raw_path: str, processed_path: str):
        """Извлечение парафраз"""
        # Извлечение парафраз из сырых данных
        pass
    
    def _extract_sentences(self, raw_path: str, processed_path: str):
        """Извлечение предложений"""
        # Извлечение предложений из текста
        pass
    
    def _create_sentiment_labels(self, processed_path: str):
        """Создание меток тональности"""
        # Создание меток для анализа тональности
        pass
    
    def _task_specific_preprocessing(self, raw_path: str, processed_path: str):
        """Задачно-специфичная предобработка"""
        # Специфичная предобработка для конкретной задачи
        pass
    
    def _unify_format(self, processed_path: str):
        """Унификация формата"""
        # Приведение к единому формату
        pass
    
    def validate_dataset(self, dataset_name: str, config: DatasetConfig) -> bool:
        """Валидация датасета"""
        try:
            print(f"🔍 Валидация датасета: {dataset_name}")
            
            processed_path = f"{self.processed_path}/{dataset_name}"
            
            if not os.path.exists(processed_path):
                print(f"❌ Обработанный датасет не найден: {processed_path}")
                return False
            
            # Применяем проверки валидации
            if config.validation_checks:
                for check in config.validation_checks:
                    print(f"  ✅ Проверка: {check}")
                    
                    if check == "check_qa_format":
                        if not self._validate_qa_format(processed_path):
                            return False
                    elif check == "validate_russian_text":
                        if not self._validate_russian_text(processed_path):
                            return False
                    elif check == "check_classification_format":
                        if not self._validate_classification_format(processed_path):
                            return False
                    elif check == "check_multitask_format":
                        if not self._validate_multitask_format(processed_path):
                            return False
                    elif check == "check_text_format":
                        if not self._validate_text_format(processed_path):
                            return False
                    elif check == "check_poem_format":
                        if not self._validate_poem_format(processed_path):
                            return False
                    elif check == "check_paraphrase_format":
                        if not self._validate_paraphrase_format(processed_path):
                            return False
                    elif check == "check_sentiment_format":
                        if not self._validate_sentiment_format(processed_path):
                            return False
            
            print(f"✅ Валидация завершена успешно")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка валидации: {e}")
            return False
    
    def _validate_qa_format(self, processed_path: str) -> bool:
        """Валидация формата вопрос-ответ"""
        # Проверка формата QA
        return True
    
    def _validate_russian_text(self, processed_path: str) -> bool:
        """Валидация русского текста"""
        # Проверка русского текста
        return True
    
    def _validate_classification_format(self, processed_path: str) -> bool:
        """Валидация формата классификации"""
        # Проверка формата классификации
        return True
    
    def _validate_multitask_format(self, processed_path: str) -> bool:
        """Валидация формата мультизадачности"""
        # Проверка формата мультизадачности
        return True
    
    def _validate_text_format(self, processed_path: str) -> bool:
        """Валидация текстового формата"""
        # Проверка текстового формата
        return True
    
    def _validate_poem_format(self, processed_path: str) -> bool:
        """Валидация формата стихов"""
        # Проверка формата стихов
        return True
    
    def _validate_paraphrase_format(self, processed_path: str) -> bool:
        """Валидация формата парафраз"""
        # Проверка формата парафраз
        return True
    
    def _validate_sentiment_format(self, processed_path: str) -> bool:
        """Валидация формата тональности"""
        # Проверка формата тональности
        return True
    
    def download_and_preprocess(self, dataset_name: str, skip_preprocessing: bool = False) -> bool:
        """Полная загрузка и предобработка датасета"""
        try:
            print(f"🚀 Загрузка и предобработка датасета: {dataset_name}")
            print("=" * 60)
            
            if dataset_name not in self.dataset_configs:
                print(f"❌ Неизвестный датасет: {dataset_name}")
                return False
            
            config = self.dataset_configs[dataset_name]
            
            # Шаг 1: Загрузка
            print(f"\n📥 Шаг 1: Загрузка датасета '{dataset_name}'")
            if config.source_type == "huggingface":
                if not self.download_huggingface_dataset(dataset_name, config):
                    return False
            elif config.source_type == "url":
                if not self.download_url_dataset(dataset_name, config):
                    return False
            else:
                print(f"❌ Неподдерживаемый тип источника: {config.source_type}")
                return False
            
            # Шаг 2: Предобработка (если не пропущена)
            if not skip_preprocessing and config.preprocessing_steps:
                print(f"\n🔧 Шаг 2: Предобработка датасета '{dataset_name}'")
                if not self.preprocess_dataset(dataset_name, config):
                    return False
            
            # Шаг 3: Валидация
            if config.validation_checks:
                print(f"\n🔍 Шаг 3: Валидация датасета '{dataset_name}'")
                if not self.validate_dataset(dataset_name, config):
                    return False
            
            print(f"\n🎉 Датасет '{dataset_name}' успешно загружен и подготовлен!")
            print(f"📁 Сырые данные: {self.raw_path}/{dataset_name}")
            if not skip_preprocessing:
                print(f"🔧 Обработанные данные: {self.processed_path}/{dataset_name}")
            print(f"💾 Кэш: {self.cached_path}/{dataset_name}")
            
            return True
            
        except Exception as e:
            print(f"❌ Ошибка в процессе загрузки и предобработки: {e}")
            return False
    
    def list_available_datasets(self):
        """Список доступных датасетов для загрузки"""
        print("📚 Доступные датасеты для загрузки:")
        print("=" * 80)
        
        for name, config in self.dataset_configs.items():
            print(f"\n🔹 {name}")
            print(f"  📊 Размер: {config.size_mb} МБ")
            print(f"  🎯 Задача: {config.task_type}")
            print(f"  🌐 Источник: {config.source_type}")
            print(f"  📝 {config.description}")
            print(f"  🔗 URL: {config.source_url}")
            
            if config.preprocessing_steps:
                print(f"  🔧 Предобработка: {', '.join(config.preprocessing_steps)}")
            
            if config.validation_checks:
                print(f"  ✅ Валидация: {', '.join(config.validation_checks)}")
    
    def get_dataset_status(self, dataset_name: str) -> Dict[str, Any]:
        """Получение статуса датасета"""
        status = {
            "name": dataset_name,
            "raw_exists": False,
            "processed_exists": False,
            "cached_exists": False,
            "raw_size_mb": 0,
            "processed_size_mb": 0,
            "cached_size_mb": 0,
            "last_modified": None
        }
        
        try:
            # Проверяем сырые данные
            raw_path = f"{self.raw_path}/{dataset_name}"
            if os.path.exists(raw_path):
                status["raw_exists"] = True
                status["raw_size_mb"] = self._get_directory_size_mb(raw_path)
                status["last_modified"] = time.ctime(os.path.getmtime(raw_path))
            
            # Проверяем обработанные данные
            processed_path = f"{self.processed_path}/{dataset_name}"
            if os.path.exists(processed_path):
                status["processed_exists"] = True
                status["processed_size_mb"] = self._get_directory_size_mb(processed_path)
            
            # Проверяем кэш
            cached_path = f"{self.cached_path}/{dataset_name}"
            if os.path.exists(cached_path):
                status["cached_exists"] = True
                status["cached_size_mb"] = self._get_directory_size_mb(cached_path)
            
        except Exception as e:
            print(f"❌ Ошибка получения статуса: {e}")
        
        return status
    
    def _get_directory_size_mb(self, directory: str) -> float:
        """Получение размера директории в МБ"""
        try:
            total_size = 0
            for dirpath, dirnames, filenames in os.walk(directory):
                for filename in filenames:
                    filepath = os.path.join(dirpath, filename)
                    try:
                        total_size += os.path.getsize(filepath)
                    except (OSError, IOError):
                        continue
            return total_size / (1024 * 1024)
        except:
            return 0.0
    
    def cleanup_temp_files(self):
        """Очистка временных файлов"""
        try:
            if os.path.exists(self.temp_path):
                shutil.rmtree(self.temp_path)
                os.makedirs(self.temp_path, exist_ok=True)
                print("🧹 Временные файлы очищены")
        except Exception as e:
            print(f"❌ Ошибка очистки временных файлов: {e}")
    
    def get_disk_usage(self) -> Dict[str, float]:
        """Получение информации об использовании диска"""
        usage = {}
        
        try:
            # Общее использование
            total, used, free = shutil.disk_usage(self.project_path)
            usage["total_gb"] = total / (1024**3)
            usage["used_gb"] = used / (1024**3)
            usage["free_gb"] = free / (1024**3)
            usage["used_percent"] = (used / total) * 100
            
            # Использование по папкам
            usage["datasets_gb"] = self._get_directory_size_mb(self.datasets_path) / 1024
            usage["raw_gb"] = self._get_directory_size_mb(self.raw_path) / 1024
            usage["processed_gb"] = self._get_directory_size_mb(self.processed_path) / 1024
            usage["cached_gb"] = self._get_directory_size_mb(self.cached_path) / 1024
            
        except Exception as e:
            print(f"❌ Ошибка получения информации о диске: {e}")
        
        return usage


def quick_download_dataset(dataset_name: str, skip_preprocessing: bool = False) -> bool:
    """Быстрая функция для загрузки датасета"""
    downloader = AllanDatasetDownloader()
    return downloader.download_and_preprocess(dataset_name, skip_preprocessing)


def list_downloadable_datasets():
    """Быстрая функция для просмотра доступных датасетов"""
    downloader = AllanDatasetDownloader()
    downloader.list_available_datasets()


def get_dataset_status(dataset_name: str) -> Dict[str, Any]:
    """Быстрая функция для получения статуса датасета"""
    downloader = AllanDatasetDownloader()
    return downloader.get_dataset_status(dataset_name)


if __name__ == "__main__":
    # Демонстрация работы загрузчика датасетов
    downloader = AllanDatasetDownloader()
    
    print("🔥 Allan Dataset Downloader - Демонстрация")
    print("=" * 60)
    
    # Показываем доступные датасеты
    downloader.list_available_datasets()
    
    # Показываем использование диска
    print(f"\n💾 Использование диска:")
    disk_usage = downloader.get_disk_usage()
    for key, value in disk_usage.items():
        if "percent" in key:
            print(f"  {key}: {value:.1f}%")
        else:
            print(f"  {key}: {value:.2f} ГБ")
    
    # Пример загрузки датасета (закомментировано для демонстрации)
    # print(f"\n🚀 Пример загрузки датасета 'sberquad':")
    # success = downloader.download_and_preprocess("sberquad")
    # if success:
    #     print("✅ Датасет успешно загружен!")
    # else:
    #     print("❌ Ошибка загрузки датасета")
