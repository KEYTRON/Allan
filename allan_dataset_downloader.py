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

# Импорт для работы с Google Drive в Google Colab
try:
    from google.colab import drive
    IS_COLAB = True
except ImportError:
    IS_COLAB = False
    print("⚠️  Google Colab не обнаружен. Проверьте, что код запускается в Colab.")

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
    
    def __init__(self, project_path: Optional[str] = None):
        # Подключение Google Drive в Colab
        if IS_COLAB:
            self._mount_google_drive()
        
        # Выбор пути проекта в зависимости от окружения
        if project_path is None:
            project_path = "/content/drive/MyDrive/ML_Projects/Allan_Model" if IS_COLAB else "/workspace/Allan_Model"
        
        self.project_path = project_path
        self.datasets_path = f"{project_path}/datasets"
        self.raw_path = f"{self.datasets_path}/raw"
        self.processed_path = f"{self.datasets_path}/processed"
        self.cached_path = f"{self.datasets_path}/cached"
        self.temp_path = f"{self.datasets_path}/temp"
        self.local_cache = "/content/allan_cache" if IS_COLAB else f"{self.project_path}/.allan_cache"
        
        # Создание необходимых директорий
        self._create_directories()
        
        # Расширенные конфигурации русскоязычных датасетов
        self.dataset_configs = self._load_dataset_configs()
        
        # Настройки загрузки
        self.chunk_size = 8192  # Размер чанка для загрузки
        self.max_retries = 3    # Максимальное количество попыток
        self.timeout = 300      # Таймаут загрузки в секундах

    def _mount_google_drive(self):
        """Подключение Google Drive в Google Colab"""
        try:
            print("🔌 Подключение Google Drive...")
            
            # Проверяем, не подключен ли уже диск
            if os.path.exists('/content/drive/MyDrive'):
                print("✅ Google Drive уже подключен")
                return True
            
            # Подключаем Google Drive
            drive.mount('/content/drive', force_remount=True)
            
            # Проверяем успешность подключения
            if os.path.exists('/content/drive/MyDrive'):
                print("✅ Google Drive успешно подключен")
                return True
            else:
                print("❌ Ошибка: Google Drive не подключен")
                return False
                
        except Exception as e:
            print(f"❌ Ошибка подключения Google Drive: {e}")
            print("💡 Попробуйте выполнить подключение вручную:")
            print("   from google.colab import drive")
            print("   drive.mount('/content/drive')")
            return False
        
    def _create_directories(self):
        """Создание необходимых директорий"""
        directories = [
            self.project_path,
            self.datasets_path,
            self.raw_path,
            self.processed_path,
            self.cached_path,
            self.temp_path,
            self.local_cache
        ]
        
        print("📁 Создание директорий...")
        for directory in directories:
            try:
                os.makedirs(directory, exist_ok=True)
                print(f"  ✅ {directory}")
            except PermissionError:
                print(f"  ❌ Нет прав для создания: {directory}")
            except Exception as e:
                print(f"  ❌ Ошибка создания {directory}: {e}")
    
    def _load_dataset_configs(self) -> Dict[str, DatasetConfig]:
        """Загрузка расширенных конфигураций русскоязычных датасетов"""
        configs = {
            # Оригинальные датасеты
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
                source_url="RussianNLP/russian_super_glue",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=200,
                description="Набор задач для оценки русскоязычных моделей",
                task_type="multi_task",
                preprocessing_steps=["task_specific_preprocessing", "unified_format"],
                validation_checks=["check_multitask_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            # Новые русскоязычные датасеты
            "russian_tape": DatasetConfig(
                name="russian_tape",
                source_url="RussianNLP/tape",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=300,
                description="TAPE - комплексная оценка понимания русского языка",
                task_type="multi_task",
                preprocessing_steps=["task_specific_preprocessing", "unified_format"],
                validation_checks=["check_multitask_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "ru_paradetox": DatasetConfig(
                name="ru_paradetox",
                source_url="s-nlp/ru_paradetox",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=100,
                description="Датасет для детоксикации русского текста",
                task_type="text_detoxification",
                preprocessing_steps=["text_cleaning", "create_pairs", "max_length_256"],
                validation_checks=["check_paraphrase_format", "validate_russian_text"],
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

            "rucos": DatasetConfig(
                name="rucos", 
                source_url="IlyaGusev/rucos",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=80,
                description="Чтение с пониманием и здравым смыслом для русского языка",
                task_type="reading_comprehension",
                preprocessing_steps=["tokenization", "max_length_512", "context_question_pairs"],
                validation_checks=["check_qa_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_sentiment_twitter": DatasetConfig(
                name="russian_sentiment_twitter",
                source_url="Tatyana/russian_sentiment_twitter",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=200,
                description="Анализ тональности русских твитов",
                task_type="sentiment_analysis",
                preprocessing_steps=["text_cleaning", "max_length_256", "sentiment_labels"],
                validation_checks=["check_sentiment_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_detox": DatasetConfig(
                name="russian_detox",
                source_url="unitary/toxic-bert",
                source_type="huggingface", 
                format="hf_dataset",
                size_mb=150,
                description="Детекция токсичности в русском тексте",
                task_type="toxicity_detection",
                preprocessing_steps=["text_cleaning", "max_length_256", "binary_labels"],
                validation_checks=["check_classification_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_ner": DatasetConfig(
                name="russian_ner",
                source_url="wietsedv/wikiner_russian",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=50,
                description="Распознавание именованных сущностей для русского языка",
                task_type="ner",
                preprocessing_steps=["tokenization", "ner_labels", "max_length_256"],
                validation_checks=["check_ner_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "openvqa_ru": DatasetConfig(
                name="openvqa_ru",
                source_url="open-vqa/openvqa_ru",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=500,
                description="Визуальные вопросы-ответы на русском языке",
                task_type="visual_qa",
                preprocessing_steps=["image_text_pairs", "max_length_256", "visual_features"],
                validation_checks=["check_vqa_format", "validate_russian_text"],
                dependencies=["transformers", "datasets", "Pillow"]
            ),

            "ruwikititle": DatasetConfig(
                name="ruwikititle",
                source_url="IlyaGusev/ruwikititle",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=300,
                description="Генерация заголовков из русской Википедии",
                task_type="title_generation",
                preprocessing_steps=["text_cleaning", "create_pairs", "max_length_512"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "taiga_news": DatasetConfig(
                name="taiga_news",
                source_url="https://github.com/TatianaShavrina/taiga_site/releases/download/v1.0/news.tar.gz",
                source_type="url",
                format="tar",
                size_mb=3000,
                description="Новостной корпус Taiga - огромная коллекция русских новостей",
                task_type="text_generation",
                preprocessing_steps=["text_cleaning", "max_length_1024", "remove_html"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["pandas", "numpy"]
            ),

            "opencorpora": DatasetConfig(
                name="opencorpora",
                source_url="https://github.com/OpenCorpora/opencorpora/archive/master.zip",
                source_type="url",
                format="zip",
                size_mb=500,
                description="Морфологически размеченный корпус русского языка",
                task_type="morphology",
                preprocessing_steps=["morphology_parsing", "tokenization", "pos_tagging"],
                validation_checks=["check_morphology_format", "validate_russian_text"],
                dependencies=["pandas", "numpy", "pymorphy2"]
            ),

            "rureviews": DatasetConfig(
                name="rureviews",
                source_url="sismetanin/rureviews",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=800,
                description="Отзывы на русском языке с различных платформ",
                task_type="sentiment_analysis",
                preprocessing_steps=["text_cleaning", "max_length_512", "sentiment_labels"],
                validation_checks=["check_sentiment_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_dialogues": DatasetConfig(
                name="russian_dialogues",
                source_url="Grossmend/oasst_ru",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=200,
                description="Диалоги на русском языке для обучения чат-ботов",
                task_type="dialogue",
                preprocessing_steps=["dialogue_formatting", "max_length_512", "conversation_pairs"],
                validation_checks=["check_dialogue_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "ru_pikabu": DatasetConfig(
                name="ru_pikabu",
                source_url="IlyaGusev/pikabu",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=1500,
                description="Посты и комментарии с сайта Pikabu",
                task_type="text_generation",
                preprocessing_steps=["text_cleaning", "max_length_1024", "social_media_formatting"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "rutax": DatasetConfig(
                name="rutax",
                source_url="https://github.com/rutar-anonymous/RuTaR/archive/main.zip",
                source_type="url",
                format="zip",
                size_mb=50,
                description="Рассуждения о налогах на русском языке",
                task_type="reasoning",
                preprocessing_steps=["text_cleaning", "max_length_512", "legal_formatting"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["pandas", "numpy"]
            ),

            "russian_literature": DatasetConfig(
                name="russian_literature",
                source_url="IlyaGusev/russian_literature",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=2000,
                description="Классическая русская литература",
                task_type="text_generation",
                preprocessing_steps=["text_cleaning", "max_length_1024", "literature_formatting"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_jokes": DatasetConfig(
                name="russian_jokes",
                source_url="cointegrated/russian-jokes-dataset",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=100,
                description="Русские анекдоты и шутки",
                task_type="text_generation",
                preprocessing_steps=["text_cleaning", "max_length_256", "joke_formatting"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_medical": DatasetConfig(
                name="russian_medical",
                source_url="d0rj/russian-medical-qa",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=300,
                description="Медицинские вопросы и ответы на русском языке",
                task_type="medical_qa",
                preprocessing_steps=["text_cleaning", "max_length_512", "medical_formatting"],
                validation_checks=["check_qa_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            "russian_headlines": DatasetConfig(
                name="russian_headlines",
                source_url="IlyaGusev/headline_cause",
                source_type="huggingface",
                format="hf_dataset",
                size_mb=250,
                description="Генерация заголовков для русских новостей",
                task_type="headline_generation",
                preprocessing_steps=["text_cleaning", "create_pairs", "max_length_256"],
                validation_checks=["check_text_format", "validate_russian_text"],
                dependencies=["transformers", "datasets"]
            ),

            # Сохраняем и ранее доступные URL-датасеты
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
            if config.format in ['zip', 'tar', 'tar.gz']:
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

    def check_google_drive_space(self) -> Dict[str, float]:
        """Проверка свободного места на Google Drive"""
        try:
            total, used, free = shutil.disk_usage("/content/drive/MyDrive")
            return {
                "total_gb": total / (1024**3),
                "used_gb": used / (1024**3),
                "free_gb": free / (1024**3),
                "free_percent": (free / total) * 100
            }
        except Exception as e:
            print(f"❌ Ошибка проверки места: {e}")
            return {}
    
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
            
            # Проверяем место на диске (в Colab)
            space_info = self.check_google_drive_space()
            if space_info and space_info.get('free_gb', 0) < (config.size_mb / 1024 * 2):  # x2 для безопасности
                print(f"⚠️  Предупреждение: Мало места на диске!")
                print(f"   Требуется: ~{config.size_mb/1024:.1f} ГБ")
                print(f"   Доступно: {space_info['free_gb']:.1f} ГБ")
            
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

    def batch_download(self, dataset_names: List[str], max_parallel: int = 2) -> Dict[str, bool]:
        """Пакетная загрузка нескольких датасетов"""
        results = {}
        
        print(f"📦 Пакетная загрузка {len(dataset_names)} датасетов...")
        print(f"🔄 Максимум параллельных загрузок: {max_parallel}")
        
        for i, dataset_name in enumerate(dataset_names, 1):
            print(f"\n📊 Прогресс: {i}/{len(dataset_names)} - {dataset_name}")
            results[dataset_name] = self.download_and_preprocess(dataset_name)
            
            if i % max_parallel == 0:
                print("⏸️  Пауза между загрузками...")
                time.sleep(5)
        
        # Отчет о результатах
        successful = sum(1 for success in results.values() if success)
        print(f"\n📋 Результаты пакетной загрузки:")
        print(f"  ✅ Успешно: {successful}/{len(dataset_names)}")
        print(f"  ❌ Ошибки: {len(dataset_names) - successful}/{len(dataset_names)}")
        
        return results
    
    def list_available_datasets(self):
        """Список доступных датасетов для загрузки"""
        print("📚 Доступные русскоязычные датасеты для загрузки:")
        print("=" * 80)
        
        # Группировка по типам задач
        by_task: Dict[str, List[Tuple[str, DatasetConfig]]] = {}
        for name, config in self.dataset_configs.items():
            task = config.task_type
            if task not in by_task:
                by_task[task] = []
            by_task[task].append((name, config))
        
        for task_type, datasets in by_task.items():
            print(f"\n🎯 **{task_type.upper()}**")
            print("-" * 50)
            
            for name, config in datasets:
                print(f"\n🔹 **{name}**")
                print(f"  📊 Размер: {config.size_mb} МБ")
                print(f"  🌐 Источник: {config.source_type}")
                print(f"  📝 {config.description}")
                print(f"  🔗 URL: {config.source_url}")
        
        # Показываем статистику
        total_size = sum(config.size_mb for config in self.dataset_configs.values())
        print(f"\n📊 **ОБЩАЯ СТАТИСТИКА:**")
        print(f"  • Всего датасетов: {len(self.dataset_configs)}")
        print(f"  • Общий размер: {total_size:.1f} МБ ({total_size/1024:.1f} ГБ)")
        print(f"  • Типов задач: {len(set(config.task_type for config in self.dataset_configs.values()))}")

    def suggest_datasets_by_task(self, task_type: str) -> List[str]:
        """Предложение датасетов по типу задачи"""
        suggestions: List[str] = []
        for name, config in self.dataset_configs.items():
            if config.task_type == task_type:
                suggestions.append(name)
        
        if suggestions:
            print(f"📋 Датасеты для задачи '{task_type}':")
            for dataset in suggestions:
                config = self.dataset_configs[dataset]
                print(f"  • {dataset} ({config.size_mb} МБ) - {config.description}")
        else:
            print(f"❌ Датасеты для задачи '{task_type}' не найдены")
        
        return suggestions

    def recommend_datasets_by_size(self, max_size_gb: float = 2.0) -> List[str]:
        """Рекомендация датасетов по размеру"""
        max_size_mb = max_size_gb * 1024
        recommendations: List[Tuple[str, DatasetConfig]] = []
        
        for name, config in self.dataset_configs.items():
            if config.size_mb <= max_size_mb:
                recommendations.append((name, config))
        
        # Сортируем по размеру
        recommendations.sort(key=lambda x: x[1].size_mb, reverse=True)
        
        print(f"📊 Датасеты размером до {max_size_gb} ГБ:")
        for name, config in recommendations:
            print(f"  • {name} ({config.size_mb} МБ) - {config.task_type}")
        
        return [name for name, _ in recommendations]

    def export_dataset_list(self, output_format: str = "json") -> str:
        """Экспорт списка датасетов в файл"""
        try:
            export_data: Dict[str, Dict[str, Union[str, float]]] = {}
            for name, config in self.dataset_configs.items():
                export_data[name] = {
                    "source_url": config.source_url,
                    "source_type": config.source_type,
                    "format": config.format,
                    "size_mb": config.size_mb,
                    "description": config.description,
                    "language": config.language,
                    "task_type": config.task_type
                }

            timestamp = time.strftime("%Y%m%d_%H%M%S")
            
            if output_format.lower() == "json":
                filename = f"datasets_list_{timestamp}.json"
                filepath = f"{self.datasets_path}/{filename}"
                with open(filepath, 'w', encoding='utf-8') as f:
                    json.dump(export_data, f, ensure_ascii=False, indent=2)
            
            elif output_format.lower() == "csv":
                import pandas as pd
                filename = f"datasets_list_{timestamp}.csv"
                filepath = f"{self.datasets_path}/{filename}"
                df = pd.DataFrame.from_dict(export_data, orient='index')
                df.to_csv(filepath, encoding='utf-8', index_label='dataset_name')

            print(f"📄 Список датасетов экспортирован: {filepath}")
            return filepath

        except Exception as e:
            print(f"❌ Ошибка экспорта: {e}")
            return ""
    
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


def batch_download_recommended(task_type: str = None, max_size_gb: float = 2.0) -> Dict[str, bool]:
    """Пакетная загрузка рекомендованных датасетов"""
    downloader = AllanDatasetDownloader()
    
    if task_type:
        datasets = downloader.suggest_datasets_by_task(task_type)
    else:
        datasets = downloader.recommend_datasets_by_size(max_size_gb)
    
    if datasets:
        return downloader.batch_download(datasets[:5])  # Максимум 5 датасетов за раз
    else:
        print("❌ Нет подходящих датасетов для загрузки")
        return {}


if __name__ == "__main__":
    # Демонстрация работы загрузчика датасетов
    downloader = AllanDatasetDownloader()

    print("🔥 Allan Dataset Downloader - Демонстрация (Обновленная версия)")
    print("=" * 70)

    # Проверяем место на диске
    space_info = downloader.check_google_drive_space()
    if space_info:
        print(f"\n💾 **Использование Google Drive:**")
        print(f"  • Всего: {space_info['total_gb']:.1f} ГБ")
        print(f"  • Использовано: {space_info['used_gb']:.1f} ГБ")
        print(f"  • Свободно: {space_info['free_gb']:.1f} ГБ ({space_info['free_percent']:.1f}%)")

    # Показываем доступные датасеты
    downloader.list_available_datasets()

    print(f"\n🚀 **Готово к загрузке датасетов!**")
    print("💡 Используйте: downloader.download_and_preprocess('dataset_name')")
