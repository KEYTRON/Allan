#!/usr/bin/env python3
"""
Allan Drive Structure Creator
Создание оптимальной структуры папок на Google Drive для проекта Allan
"""

import os
import json
from pathlib import Path
from typing import Dict, List
from datetime import datetime

class AllanDriveStructure:
    """Создатель структуры папок для проекта Allan на Google Drive"""
    
    def __init__(self, base_path: str = "/content/drive/MyDrive/ML_Projects/Allan_Model"):
        self.base_path = base_path
        self.structure = self._define_structure()
    
    def _define_structure(self) -> Dict:
        """Определение полной структуры папок проекта"""
        return {
            "datasets": {
                "description": "Все датасеты для обучения Allan",
                "subfolders": {
                    "raw": {
                        "description": "Исходные необработанные датасеты",
                        "subfolders": {
                            "sberquad": {"description": "SberQuAD датасет (вопрос-ответ)"},
                            "rucola": {"description": "RuCoLA датасет (лингвистическая приемлемость)"},
                            "russian_superglue": {"description": "Russian SuperGLUE бенчмарк"},
                            "lenta_news": {"description": "Новости Lenta.ru для генерации текста"},
                            "russian_poems": {"description": "Русская поэзия для стилистики"},
                            "custom": {"description": "Пользовательские датасеты"}
                        }
                    },
                    "processed": {
                        "description": "Обработанные и токенизированные датасеты",
                        "subfolders": {
                            "tokenized": {"description": "Токенизированные датасеты"},
                            "filtered": {"description": "Отфильтрованные датасеты"},
                            "augmented": {"description": "Дополненные данные"}
                        }
                    },
                    "cached": {
                        "description": "Кэшированные датасеты для быстрого доступа",
                        "subfolders": {
                            "hf_cache": {"description": "Кэш Hugging Face датасетов"},
                            "preprocessed": {"description": "Предобработанные данные"}
                        }
                    },
                    "splits": {
                        "description": "Разделения датасетов на train/val/test",
                        "subfolders": {
                            "train": {"description": "Тренировочные данные"},
                            "validation": {"description": "Валидационные данные"},
                            "test": {"description": "Тестовые данные"}
                        }
                    }
                }
            },
            "models": {
                "description": "Модели Allan и связанные артефакты",
                "subfolders": {
                    "checkpoints": {
                        "description": "Чекпоинты во время обучения",
                        "subfolders": {
                            "epoch_checkpoints": {"description": "Чекпоинты по эпохам"},
                            "best_checkpoints": {"description": "Лучшие чекпоинты по метрикам"},
                            "backup_checkpoints": {"description": "Резервные копии чекпоинтов"}
                        }
                    },
                    "final": {
                        "description": "Финальные обученные модели",
                        "subfolders": {
                            "allan_v1": {"description": "Allan версия 1.0"},
                            "allan_v2": {"description": "Allan версия 2.0"},
                            "experimental": {"description": "Экспериментальные версии"}
                        }
                    },
                    "tokenizers": {
                        "description": "Токенизаторы для разных версий Allan",
                        "subfolders": {
                            "custom_tokenizers": {"description": "Пользовательские токенизаторы"},
                            "pretrained_tokenizers": {"description": "Предобученные токенизаторы"}
                        }
                    },
                    "base_models": {
                        "description": "Базовые модели для файн-тюнинга",
                        "subfolders": {
                            "rubert": {"description": "RuBERT модели"},
                            "rugpt": {"description": "ruGPT модели"},
                            "other": {"description": "Другие базовые модели"}
                        }
                    }
                }
            },
            "configs": {
                "description": "Конфигурационные файлы",
                "subfolders": {
                    "training": {"description": "Конфиги для обучения"},
                    "model": {"description": "Архитектурные конфиги моделей"},
                    "data": {"description": "Конфиги обработки данных"},
                    "evaluation": {"description": "Конфиги для оценки"}
                }
            },
            "scripts": {
                "description": "Скрипты для различных задач",
                "subfolders": {
                    "training": {"description": "Скрипты обучения"},
                    "evaluation": {"description": "Скрипты оценки"},
                    "data_processing": {"description": "Обработка данных"},
                    "utils": {"description": "Утилиты"}
                }
            },
            "notebooks": {
                "description": "Jupyter ноутбуки для экспериментов",
                "subfolders": {
                    "experiments": {"description": "Экспериментальные ноутбуки"},
                    "analysis": {"description": "Анализ данных и результатов"},
                    "demos": {"description": "Демонстрационные ноутбуки"},
                    "tutorials": {"description": "Обучающие материалы"}
                }
            },
            "logs": {
                "description": "Логи обучения и экспериментов",
                "subfolders": {
                    "tensorboard": {
                        "description": "Логи TensorBoard",
                        "subfolders": {
                            "training": {"description": "Логи обучения"},
                            "validation": {"description": "Логи валидации"}
                        }
                    },
                    "wandb": {
                        "description": "Weights & Biases логи",
                        "subfolders": {
                            "runs": {"description": "Отдельные запуски"},
                            "sweeps": {"description": "Sweep эксперименты"}
                        }
                    },
                    "training_logs": {"description": "Текстовые логи обучения"},
                    "error_logs": {"description": "Логи ошибок"}
                }
            },
            "results": {
                "description": "Результаты экспериментов и оценки",
                "subfolders": {
                    "metrics": {"description": "Метрики производительности"},
                    "predictions": {"description": "Предсказания модели"},
                    "evaluations": {"description": "Результаты оценки"},
                    "comparisons": {"description": "Сравнения моделей"},
                    "reports": {"description": "Отчеты и анализы"}
                }
            },
            "cache": {
                "description": "Различные кэши для ускорения работы",
                "subfolders": {
                    "huggingface": {"description": "Кэш Hugging Face"},
                    "transformers": {"description": "Кэш Transformers"},
                    "datasets": {"description": "Кэш датасетов"},
                    "torch": {"description": "Кэш PyTorch"},
                    "temp": {"description": "Временные файлы"}
                }
            },
            "docs": {
                "description": "Документация проекта",
                "subfolders": {
                    "model_docs": {"description": "Документация модели"},
                    "api_docs": {"description": "API документация"},
                    "user_guides": {"description": "Руководства пользователя"},
                    "research_notes": {"description": "Исследовательские заметки"}
                }
            },
            "tools": {
                "description": "Инструменты и утилиты",
                "subfolders": {
                    "monitoring": {"description": "Инструменты мониторинга"},
                    "visualization": {"description": "Визуализация"},
                    "deployment": {"description": "Деплоймент утилиты"},
                    "benchmarking": {"description": "Бенчмарки"}
                }
            }
        }
    
    def create_folder_with_readme(self, folder_path: str, description: str):
        """Создание папки с README файлом"""
        try:
            # Создание папки
            os.makedirs(folder_path, exist_ok=True)
            
            # Создание README файла
            readme_path = os.path.join(folder_path, "README.md")
            if not os.path.exists(readme_path):
                with open(readme_path, 'w', encoding='utf-8') as f:
                    folder_name = os.path.basename(folder_path)
                    f.write(f"# {folder_name.upper()}\n\n")
                    f.write(f"{description}\n\n")
                    f.write(f"Создано: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
                    f.write(f"Проект: Allan Model\n\n")
                    f.write("## Содержимое\n\n")
                    f.write("_Папка пока пуста. Файлы будут добавлены в процессе работы._\n")
            
            return True
            
        except Exception as e:
            print(f"❌ Ошибка создания папки {folder_path}: {e}")
            return False
    
    def create_structure_recursive(self, structure: Dict, current_path: str, level: int = 0):
        """Рекурсивное создание структуры папок"""
        created_count = 0
        
        for name, info in structure.items():
            folder_path = os.path.join(current_path, name)
            description = info.get("description", f"Папка {name}")
            
            # Создание папки с README
            if self.create_folder_with_readme(folder_path, description):
                created_count += 1
                indent = "  " * level
                print(f"{indent}📁 {name} - {description}")
            
            # Рекурсивное создание подпапок
            if "subfolders" in info:
                sub_created = self.create_structure_recursive(
                    info["subfolders"], 
                    folder_path, 
                    level + 1
                )
                created_count += sub_created
        
        return created_count
    
    def create_project_structure(self) -> bool:
        """Создание полной структуры проекта Allan"""
        print("🚀 Создание структуры проекта Allan на Google Drive...")
        print("=" * 60)
        print(f"📍 Базовый путь: {self.base_path}")
        print()
        
        try:
            # Создание базовой папки проекта
            os.makedirs(self.base_path, exist_ok=True)
            
            # Создание главного README
            self.create_main_readme()
            
            # Создание структуры папок
            total_created = self.create_structure_recursive(self.structure, self.base_path)
            
            # Создание дополнительных файлов
            self.create_project_files()
            
            print(f"\n✅ Структура создана успешно!")
            print(f"📊 Создано папок: {total_created}")
            print(f"📁 Базовый путь: {self.base_path}")
            
            return True
            
        except Exception as e:
            print(f"❌ Ошибка создания структуры: {e}")
            return False
    
    def create_main_readme(self):
        """Создание главного README файла проекта"""
        readme_path = os.path.join(self.base_path, "README.md")
        
        content = f"""# 🔥 Allan Model Project

Проект обучения русскоязычной языковой модели Allan с использованием Google Colab и Google Drive.

## 📊 Информация о проекте

- **Создан**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
- **Платформа**: Google Colab + Google Drive
- **Язык**: Русский
- **Фреймворк**: PyTorch + Transformers

## 🎯 Цели проекта

1. Обучение высококачественной русскоязычной модели
2. Оптимизация для различных NLP задач
3. Создание эффективного пайплайна обучения
4. Максимальное использование возможностей Google Drive

## 📁 Структура проекта

```
Allan_Model/
├── datasets/          # Датасеты для обучения
├── models/            # Модели и чекпоинты
├── configs/           # Конфигурационные файлы
├── scripts/           # Скрипты обучения и оценки
├── notebooks/         # Jupyter ноутбуки
├── logs/              # Логи экспериментов
├── results/           # Результаты и метрики
├── cache/             # Кэш для ускорения
├── docs/              # Документация
└── tools/             # Утилиты и инструменты
```

## 🚀 Быстрый старт

1. Запустите настройку среды:
```python
from allan_colab_setup import setup_allan
setup_allan()
```

2. Загрузите датасет:
```python
from allan_dataset_manager import quick_load_dataset
dataset = quick_load_dataset("sberquad")
```

3. Начните обучение:
```python
# Код обучения будет добавлен позже
```

## 📚 Рекомендованные датасеты

- **SberQuAD**: Вопрос-ответ (150 МБ)
- **RuCoLA**: Лингвистическая приемлемость (50 МБ)  
- **Russian SuperGLUE**: Мульти-задачный бенчмарк (200 МБ)
- **Lenta News**: Генерация текста (2 ГБ)
- **Russian Poems**: Поэзия и стилистика (150 МБ)

## 🛠️ Инструменты

- `allan_colab_setup.py` - Автоматическая настройка среды
- `allan_dataset_manager.py` - Умное управление датасетами
- `allan_performance_optimizer.py` - Оптимизация производительности

## 📊 Мониторинг ресурсов

Используйте встроенные инструменты мониторинга для отслеживания:
- Использование RAM (12-13 ГБ доступно)
- Использование диска (80 ГБ локально)
- Использование GPU (15-16 ГБ)
- Свободное место на Drive (1.99 ТБ!)

## 🔗 Полезные ссылки

- [Google Colab](https://colab.research.google.com/)
- [Hugging Face Transformers](https://huggingface.co/transformers/)
- [PyTorch](https://pytorch.org/)

---

**Allan Model** - Создавая будущее русскоязычного NLP 🚀
"""
        
        with open(readme_path, 'w', encoding='utf-8') as f:
            f.write(content)
        
        print("📄 Главный README создан")
    
    def create_project_files(self):
        """Создание дополнительных файлов проекта"""
        
        # .gitignore для случая если проект будет версионироваться
        gitignore_path = os.path.join(self.base_path, ".gitignore")
        gitignore_content = """# Allan Model .gitignore

# Датасеты и кэш
datasets/raw/
datasets/cached/
cache/
*.arrow
*.bin

# Модели и чекпоинты  
models/checkpoints/
models/final/
*.safetensors
*.pt
*.pth

# Логи
logs/
*.log

# Временные файлы
temp/
*.tmp

# Системные файлы
.DS_Store
Thumbs.db
*.swp
*.swo

# Python
__pycache__/
*.pyc
*.pyo
*.pyd
.Python
*.so

# Jupyter
.ipynb_checkpoints/

# IDE
.vscode/
.idea/
*.sublime-*

# Окружение
.env
.venv/
"""
        
        with open(gitignore_path, 'w', encoding='utf-8') as f:
            f.write(gitignore_content)
        
        print("📄 .gitignore создан")
        
        # requirements.txt
        requirements_path = os.path.join(self.base_path, "requirements.txt")
        requirements_content = """# Allan Model Requirements

# Основные библиотеки
torch>=2.0.0
transformers[torch]>=4.35.0
datasets>=2.14.0
tokenizers>=0.14.0
accelerate>=0.24.0

# Оценка и метрики
evaluate>=0.4.0
scikit-learn>=1.3.0

# Русский язык
pymorphy2[fast]>=0.9.1
razdel>=0.5.0
sentencepiece>=0.1.99

# Логирование и мониторинг
wandb>=0.16.0
tensorboard>=2.14.0
psutil>=5.9.0
gpustat>=1.1.0

# Визуализация
matplotlib>=3.7.0
seaborn>=0.12.0

# Утилиты
tqdm>=4.65.0
numpy>=1.24.0
pandas>=2.0.0
"""
        
        with open(requirements_path, 'w', encoding='utf-8') as f:
            f.write(requirements_content)
        
        print("📄 requirements.txt создан")
        
        # Конфиг проекта в JSON
        config_path = os.path.join(self.base_path, "project_config.json")
        config_content = {
            "project_name": "Allan Model",
            "version": "1.0.0",
            "created": datetime.now().isoformat(),
            "platform": "Google Colab + Google Drive",
            "language": "Russian",
            "base_path": self.base_path,
            "drive_quota": {
                "total_gb": 2048,
                "used_gb": 11,
                "available_gb": 2037
            },
            "colab_resources": {
                "ram_gb": 13,
                "disk_gb": 80,
                "gpu_memory_gb": 16
            },
            "recommended_datasets": list(self.structure["datasets"]["subfolders"]["raw"]["subfolders"].keys())
        }
        
        with open(config_path, 'w', encoding='utf-8') as f:
            json.dump(config_content, f, indent=2, ensure_ascii=False)
        
        print("📄 project_config.json создан")
    
    def get_structure_summary(self) -> Dict:
        """Получение сводки о созданной структуре"""
        def count_folders(structure):
            count = len(structure)
            for item in structure.values():
                if "subfolders" in item:
                    count += count_folders(item["subfolders"])
            return count
        
        total_folders = count_folders(self.structure)
        
        return {
            "base_path": self.base_path,
            "total_folders": total_folders,
            "main_categories": len(self.structure),
            "created_at": datetime.now().isoformat()
        }
    
    def print_structure_tree(self, structure: Dict = None, level: int = 0):
        """Печать дерева структуры папок"""
        if structure is None:
            structure = self.structure
            print("🌳 Структура проекта Allan:")
            print("=" * 40)
        
        for name, info in structure.items():
            indent = "  " * level
            icon = "📁" if level == 0 else "└─" if level > 0 else ""
            description = info.get("description", "")
            
            print(f"{indent}{icon} {name} - {description}")
            
            if "subfolders" in info:
                self.print_structure_tree(info["subfolders"], level + 1)


def create_allan_drive_structure(base_path: str = None) -> bool:
    """Быстрая функция для создания структуры Allan на Drive"""
    if base_path is None:
        base_path = "/content/drive/MyDrive/ML_Projects/Allan_Model"
    
    creator = AllanDriveStructure(base_path)
    return creator.create_project_structure()


def print_allan_structure():
    """Быстрая функция для просмотра структуры"""
    creator = AllanDriveStructure()
    creator.print_structure_tree()


if __name__ == "__main__":
    # Демонстрация создания структуры
    creator = AllanDriveStructure()
    
    print("🔥 Allan Drive Structure Creator")
    print("=" * 50)
    
    # Показ структуры
    creator.print_structure_tree()
    
    # Создание структуры (закомментировано для безопасности)
    # creator.create_project_structure()