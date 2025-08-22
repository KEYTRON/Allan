#!/usr/bin/env python3
"""
Allan Performance Optimizer
Оптимизация производительности и мониторинг ресурсов для обучения Allan в Colab
"""

import os
import gc
import time
import psutil
import subprocess
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass
from datetime import datetime, timedelta
import warnings

@dataclass
class ResourceMetrics:
    """Метрики использования ресурсов"""
    timestamp: datetime
    cpu_percent: float
    ram_used_gb: float
    ram_total_gb: float
    ram_percent: float
    disk_used_gb: float
    disk_total_gb: float
    disk_percent: float
    gpu_used_gb: Optional[float] = None
    gpu_total_gb: Optional[float] = None
    gpu_percent: Optional[float] = None
    temperature: Optional[float] = None

class AllanPerformanceOptimizer:
    """Оптимизатор производительности для Allan"""
    
    def __init__(self):
        self.monitoring_active = False
        self.metrics_history: List[ResourceMetrics] = []
        self.optimization_applied = set()
        
        # Пороговые значения для предупреждений
        self.WARNING_THRESHOLDS = {
            "ram_percent": 85.0,      # > 85% RAM
            "disk_percent": 90.0,     # > 90% диска
            "gpu_percent": 95.0,      # > 95% GPU
            "cpu_percent": 90.0,      # > 90% CPU
        }
        
        # Критические пороги для автоматической очистки
        self.CRITICAL_THRESHOLDS = {
            "ram_percent": 95.0,
            "disk_percent": 95.0,
            "gpu_percent": 98.0,
        }
    
    def get_current_metrics(self) -> ResourceMetrics:
        """Получение текущих метрик системы"""
        # CPU и RAM
        cpu_percent = psutil.cpu_percent(interval=1)
        memory = psutil.virtual_memory()
        
        # Диск
        disk = psutil.disk_usage("/content")
        
        # GPU (если доступен)
        gpu_used_gb = gpu_total_gb = gpu_percent = None
        try:
            import torch
            if torch.cuda.is_available():
                gpu_props = torch.cuda.get_device_properties(0)
                gpu_total_gb = gpu_props.total_memory / (1024**3)
                gpu_used_gb = torch.cuda.memory_allocated(0) / (1024**3)
                gpu_percent = (gpu_used_gb / gpu_total_gb) * 100
        except:
            pass
        
        return ResourceMetrics(
            timestamp=datetime.now(),
            cpu_percent=cpu_percent,
            ram_used_gb=memory.used / (1024**3),
            ram_total_gb=memory.total / (1024**3),
            ram_percent=memory.percent,
            disk_used_gb=disk.used / (1024**3),
            disk_total_gb=disk.total / (1024**3),
            disk_percent=(disk.used / disk.total) * 100,
            gpu_used_gb=gpu_used_gb,
            gpu_total_gb=gpu_total_gb,
            gpu_percent=gpu_percent
        )
    
    def print_current_status(self):
        """Печать текущего состояния ресурсов"""
        metrics = self.get_current_metrics()
        
        print("📊 Текущее состояние ресурсов:")
        print("=" * 50)
        
        # CPU
        cpu_icon = "🔥" if metrics.cpu_percent > 80 else "⚡" if metrics.cpu_percent > 50 else "💚"
        print(f"  {cpu_icon} CPU: {metrics.cpu_percent:.1f}%")
        
        # RAM
        ram_icon = "🔴" if metrics.ram_percent > 90 else "🟡" if metrics.ram_percent > 70 else "💚"
        print(f"  {ram_icon} RAM: {metrics.ram_percent:.1f}% ({metrics.ram_used_gb:.1f}/{metrics.ram_total_gb:.1f} ГБ)")
        
        # Диск
        disk_icon = "🔴" if metrics.disk_percent > 90 else "🟡" if metrics.disk_percent > 70 else "💚"
        print(f"  {disk_icon} Диск: {metrics.disk_percent:.1f}% ({metrics.disk_used_gb:.1f}/{metrics.disk_total_gb:.1f} ГБ)")
        
        # GPU
        if metrics.gpu_percent is not None:
            gpu_icon = "🔴" if metrics.gpu_percent > 90 else "🟡" if metrics.gpu_percent > 70 else "💚"
            print(f"  {gpu_icon} GPU: {metrics.gpu_percent:.1f}% ({metrics.gpu_used_gb:.1f}/{metrics.gpu_total_gb:.1f} ГБ)")
        else:
            print("  ⚠️  GPU: Недоступен")
        
        # Предупреждения
        warnings_list = self.check_resource_warnings(metrics)
        if warnings_list:
            print("\n⚠️  Предупреждения:")
            for warning in warnings_list:
                print(f"    • {warning}")
        
        print()
    
    def check_resource_warnings(self, metrics: ResourceMetrics) -> List[str]:
        """Проверка ресурсов на превышение порогов"""
        warnings_list = []
        
        if metrics.ram_percent > self.WARNING_THRESHOLDS["ram_percent"]:
            warnings_list.append(f"Высокое использование RAM: {metrics.ram_percent:.1f}%")
        
        if metrics.disk_percent > self.WARNING_THRESHOLDS["disk_percent"]:
            warnings_list.append(f"Мало места на диске: {metrics.disk_percent:.1f}%")
        
        if metrics.cpu_percent > self.WARNING_THRESHOLDS["cpu_percent"]:
            warnings_list.append(f"Высокая загрузка CPU: {metrics.cpu_percent:.1f}%")
        
        if metrics.gpu_percent and metrics.gpu_percent > self.WARNING_THRESHOLDS["gpu_percent"]:
            warnings_list.append(f"Высокое использование GPU: {metrics.gpu_percent:.1f}%")
        
        return warnings_list
    
    def optimize_memory(self, aggressive: bool = False) -> Dict[str, bool]:
        """Оптимизация использования памяти"""
        print("🧹 Оптимизация памяти...")
        results = {}
        
        # Очистка Python garbage collector
        print("  🗑️  Очистка Python GC...")
        collected = gc.collect()
        results["gc_cleanup"] = True
        print(f"    Освобождено объектов: {collected}")
        
        # Очистка кэша PyTorch
        try:
            import torch
            if torch.cuda.is_available():
                print("  🎮 Очистка GPU кэша...")
                torch.cuda.empty_cache()
                torch.cuda.synchronize()
                results["gpu_cache_cleanup"] = True
                print("    GPU кэш очищен")
        except Exception as e:
            results["gpu_cache_cleanup"] = False
            print(f"    ❌ Ошибка очистки GPU: {e}")
        
        # Очистка кэша Hugging Face (если агрессивная очистка)
        if aggressive:
            print("  🤗 Очистка кэша Hugging Face...")
            try:
                hf_cache_dir = os.environ.get("HF_HOME", "~/.cache/huggingface")
                temp_files = []
                
                # Поиск временных файлов
                for root, dirs, files in os.walk(os.path.expanduser(hf_cache_dir)):
                    for file in files:
                        if file.startswith("tmp") or file.endswith(".tmp"):
                            temp_files.append(os.path.join(root, file))
                
                # Удаление временных файлов
                for temp_file in temp_files:
                    try:
                        os.remove(temp_file)
                    except:
                        pass
                
                results["hf_cache_cleanup"] = True
                print(f"    Удалено временных файлов: {len(temp_files)}")
                
            except Exception as e:
                results["hf_cache_cleanup"] = False
                print(f"    ❌ Ошибка очистки HF кэша: {e}")
        
        # Принудительная очистка переменных (агрессивная)
        if aggressive:
            print("  🔄 Принудительная очистка переменных...")
            # Это опасная операция, используем с осторожностью
            import sys
            module = sys.modules[__name__]
            for var_name in dir(module):
                if not var_name.startswith('_') and var_name not in ['gc', 'torch', 'os', 'psutil']:
                    try:
                        delattr(module, var_name)
                    except:
                        pass
            results["variable_cleanup"] = True
        
        print("✅ Оптимизация памяти завершена")
        return results
    
    def optimize_disk_space(self) -> Dict[str, bool]:
        """Оптимизация дискового пространства"""
        print("💾 Оптимизация дискового пространства...")
        results = {}
        
        # Очистка временных файлов
        print("  🗑️  Очистка временных файлов...")
        temp_dirs = ["/tmp", "/content/allan_cache/temp", "/content/.cache"]
        total_cleaned = 0
        
        for temp_dir in temp_dirs:
            if os.path.exists(temp_dir):
                try:
                    for root, dirs, files in os.walk(temp_dir):
                        for file in files:
                            file_path = os.path.join(root, file)
                            try:
                                file_size = os.path.getsize(file_path)
                                os.remove(file_path)
                                total_cleaned += file_size
                            except:
                                pass
                except:
                    pass
        
        results["temp_cleanup"] = True
        print(f"    Освобождено: {total_cleaned / (1024*1024):.1f} МБ")
        
        # Очистка pip кэша
        print("  📦 Очистка pip кэша...")
        try:
            subprocess.run(["pip", "cache", "purge"], 
                         capture_output=True, check=False)
            results["pip_cache_cleanup"] = True
            print("    Pip кэш очищен")
        except:
            results["pip_cache_cleanup"] = False
            print("    ❌ Ошибка очистки pip кэша")
        
        # Очистка apt кэша (если доступен)
        print("  🔧 Очистка системного кэша...")
        try:
            subprocess.run(["apt", "clean"], 
                         capture_output=True, check=False)
            results["apt_cache_cleanup"] = True
            print("    Системный кэш очищен")
        except:
            results["apt_cache_cleanup"] = False
        
        print("✅ Оптимизация диска завершена")
        return results
    
    def optimize_for_training(self) -> Dict[str, bool]:
        """Оптимизация системы для обучения модели"""
        print("🚀 Оптимизация системы для обучения Allan...")
        print("=" * 50)
        
        results = {}
        
        # 1. Настройка переменных окружения
        print("🔧 Настройка переменных окружения...")
        
        # Оптимизация PyTorch
        os.environ["PYTORCH_CUDA_ALLOC_CONF"] = "max_split_size_mb:128"
        os.environ["CUDA_LAUNCH_BLOCKING"] = "0"  # Асинхронные CUDA операции
        
        # Оптимизация Transformers
        os.environ["TRANSFORMERS_NO_ADVISORY_WARNINGS"] = "1"
        os.environ["TOKENIZERS_PARALLELISM"] = "true"
        
        # Оптимизация datasets
        os.environ["HF_DATASETS_IN_MEMORY_MAX_SIZE"] = "0"  # Не ограничивать память
        
        results["env_optimization"] = True
        print("  ✅ Переменные окружения настроены")
        
        # 2. Настройка warnings
        print("⚠️  Настройка предупреждений...")
        warnings.filterwarnings("ignore", category=UserWarning)
        warnings.filterwarnings("ignore", category=FutureWarning)
        results["warnings_optimization"] = True
        print("  ✅ Предупреждения настроены")
        
        # 3. Оптимизация памяти
        memory_results = self.optimize_memory(aggressive=False)
        results.update(memory_results)
        
        # 4. Настройка GPU (если доступен)
        try:
            import torch
            if torch.cuda.is_available():
                print("🎮 Настройка GPU...")
                
                # Включение оптимизаций
                torch.backends.cudnn.benchmark = True
                torch.backends.cudnn.enabled = True
                
                # Настройка памяти GPU
                torch.cuda.empty_cache()
                
                results["gpu_optimization"] = True
                print("  ✅ GPU оптимизирован")
        except:
            results["gpu_optimization"] = False
            print("  ❌ GPU недоступен")
        
        # 5. Проверка и рекомендации
        self.print_training_recommendations()
        
        print("\n🎉 Оптимизация для обучения завершена!")
        return results
    
    def print_training_recommendations(self):
        """Печать рекомендаций для обучения"""
        metrics = self.get_current_metrics()
        
        print("\n💡 Рекомендации для обучения:")
        print("-" * 40)
        
        # Рекомендации по batch size
        if metrics.gpu_total_gb:
            if metrics.gpu_total_gb >= 15:
                batch_size = "16-32"
                print(f"  🎯 Рекомендуемый batch size: {batch_size} (GPU: {metrics.gpu_total_gb:.1f} ГБ)")
            elif metrics.gpu_total_gb >= 12:
                batch_size = "8-16"
                print(f"  🎯 Рекомендуемый batch size: {batch_size} (GPU: {metrics.gpu_total_gb:.1f} ГБ)")
            else:
                batch_size = "4-8"
                print(f"  🎯 Рекомендуемый batch size: {batch_size} (GPU: {metrics.gpu_total_gb:.1f} ГБ)")
        
        # Рекомендации по градиентному накоплению
        print(f"  📈 Gradient accumulation steps: 2-4 (для эмуляции большего batch size)")
        
        # Рекомендации по precision
        if metrics.gpu_total_gb and metrics.gpu_total_gb >= 15:
            print(f"  🎛️  Mixed precision: fp16 или bf16 (экономия памяти)")
        else:
            print(f"  🎛️  Mixed precision: fp16 обязательно (экономия памяти)")
        
        # Рекомендации по checkpoint
        print(f"  💾 Gradient checkpointing: Включить (экономия памяти)")
        
        # Рекомендации по мониторингу
        print(f"  📊 Мониторинг: Проверяйте ресурсы каждые 100 шагов")
        
        # Рекомендации по датасетам
        available_space = metrics.disk_total_gb - metrics.disk_used_gb
        if available_space < 10:
            print(f"  ⚠️  Мало места на диске: используйте потоковую загрузку")
        else:
            print(f"  💾 Достаточно места: можно копировать датасеты локально")
    
    def monitor_training(self, duration_minutes: int = 60, check_interval: int = 30):
        """Мониторинг ресурсов во время обучения"""
        print(f"👁️  Запуск мониторинга на {duration_minutes} минут...")
        print(f"📊 Интервал проверки: {check_interval} секунд")
        
        self.monitoring_active = True
        start_time = datetime.now()
        end_time = start_time + timedelta(minutes=duration_minutes)
        
        try:
            while datetime.now() < end_time and self.monitoring_active:
                metrics = self.get_current_metrics()
                self.metrics_history.append(metrics)
                
                # Проверка критических порогов
                critical_issues = self.check_critical_thresholds(metrics)
                if critical_issues:
                    print(f"\n🚨 КРИТИЧЕСКИЕ ПРОБЛЕМЫ обнаружены в {metrics.timestamp.strftime('%H:%M:%S')}:")
                    for issue in critical_issues:
                        print(f"    • {issue}")
                    
                    # Автоматическая очистка при критических проблемах
                    self.auto_cleanup_on_critical()
                
                # Обычные предупреждения
                warnings_list = self.check_resource_warnings(metrics)
                if warnings_list:
                    print(f"\n⚠️  Предупреждения в {metrics.timestamp.strftime('%H:%M:%S')}:")
                    for warning in warnings_list:
                        print(f"    • {warning}")
                
                time.sleep(check_interval)
                
        except KeyboardInterrupt:
            print("\n⏹️  Мониторинг остановлен пользователем")
        
        self.monitoring_active = False
        print(f"\n✅ Мониторинг завершен. Собрано {len(self.metrics_history)} измерений")
    
    def check_critical_thresholds(self, metrics: ResourceMetrics) -> List[str]:
        """Проверка критических порогов"""
        critical_issues = []
        
        if metrics.ram_percent > self.CRITICAL_THRESHOLDS["ram_percent"]:
            critical_issues.append(f"КРИТИЧНО: RAM {metrics.ram_percent:.1f}% > {self.CRITICAL_THRESHOLDS['ram_percent']}%")
        
        if metrics.disk_percent > self.CRITICAL_THRESHOLDS["disk_percent"]:
            critical_issues.append(f"КРИТИЧНО: Диск {metrics.disk_percent:.1f}% > {self.CRITICAL_THRESHOLDS['disk_percent']}%")
        
        if metrics.gpu_percent and metrics.gpu_percent > self.CRITICAL_THRESHOLDS["gpu_percent"]:
            critical_issues.append(f"КРИТИЧНО: GPU {metrics.gpu_percent:.1f}% > {self.CRITICAL_THRESHOLDS['gpu_percent']}%")
        
        return critical_issues
    
    def auto_cleanup_on_critical(self):
        """Автоматическая очистка при критических проблемах"""
        print("🚨 Выполняется автоматическая очистка...")
        
        # Агрессивная очистка памяти
        self.optimize_memory(aggressive=True)
        
        # Очистка диска
        self.optimize_disk_space()
        
        print("✅ Автоматическая очистка завершена")
    
    def generate_performance_report(self) -> str:
        """Генерация отчета о производительности"""
        if not self.metrics_history:
            return "📊 Нет данных для отчета. Запустите мониторинг сначала."
        
        report = []
        report.append("📊 ОТЧЕТ О ПРОИЗВОДИТЕЛЬНОСТИ ALLAN")
        report.append("=" * 50)
        
        # Основная статистика
        total_measurements = len(self.metrics_history)
        duration = self.metrics_history[-1].timestamp - self.metrics_history[0].timestamp
        
        report.append(f"🕒 Период мониторинга: {duration}")
        report.append(f"📈 Количество измерений: {total_measurements}")
        report.append("")
        
        # Статистика по ресурсам
        ram_values = [m.ram_percent for m in self.metrics_history]
        cpu_values = [m.cpu_percent for m in self.metrics_history]
        disk_values = [m.disk_percent for m in self.metrics_history]
        
        report.append("💾 СТАТИСТИКА ИСПОЛЬЗОВАНИЯ РЕСУРСОВ:")
        report.append(f"  RAM: мин {min(ram_values):.1f}% | макс {max(ram_values):.1f}% | сред {sum(ram_values)/len(ram_values):.1f}%")
        report.append(f"  CPU: мин {min(cpu_values):.1f}% | макс {max(cpu_values):.1f}% | сред {sum(cpu_values)/len(cpu_values):.1f}%")
        report.append(f"  Диск: мин {min(disk_values):.1f}% | макс {max(disk_values):.1f}% | сред {sum(disk_values)/len(disk_values):.1f}%")
        
        # GPU статистика (если доступна)
        gpu_values = [m.gpu_percent for m in self.metrics_history if m.gpu_percent is not None]
        if gpu_values:
            report.append(f"  GPU: мин {min(gpu_values):.1f}% | макс {max(gpu_values):.1f}% | сред {sum(gpu_values)/len(gpu_values):.1f}%")
        
        report.append("")
        
        # Предупреждения и рекомендации
        warning_count = sum(1 for m in self.metrics_history if self.check_resource_warnings(m))
        critical_count = sum(1 for m in self.metrics_history if self.check_critical_thresholds(m))
        
        report.append("⚠️  ПРЕДУПРЕЖДЕНИЯ И ПРОБЛЕМЫ:")
        report.append(f"  Предупреждений: {warning_count}/{total_measurements} ({warning_count/total_measurements*100:.1f}%)")
        report.append(f"  Критических проблем: {critical_count}/{total_measurements} ({critical_count/total_measurements*100:.1f}%)")
        
        return "\n".join(report)
    
    def save_metrics_to_drive(self, drive_path: str):
        """Сохранение метрик на Google Drive"""
        if not self.metrics_history:
            print("❌ Нет данных для сохранения")
            return False
        
        try:
            import json
            
            # Подготовка данных
            data = {
                "project": "Allan Model",
                "monitoring_session": {
                    "start_time": self.metrics_history[0].timestamp.isoformat(),
                    "end_time": self.metrics_history[-1].timestamp.isoformat(),
                    "total_measurements": len(self.metrics_history)
                },
                "metrics": []
            }
            
            for metric in self.metrics_history:
                data["metrics"].append({
                    "timestamp": metric.timestamp.isoformat(),
                    "cpu_percent": metric.cpu_percent,
                    "ram_percent": metric.ram_percent,
                    "ram_used_gb": metric.ram_used_gb,
                    "disk_percent": metric.disk_percent,
                    "gpu_percent": metric.gpu_percent,
                    "gpu_used_gb": metric.gpu_used_gb
                })
            
            # Сохранение
            filename = f"allan_performance_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
            filepath = os.path.join(drive_path, filename)
            
            with open(filepath, 'w') as f:
                json.dump(data, f, indent=2)
            
            print(f"✅ Метрики сохранены: {filepath}")
            return True
            
        except Exception as e:
            print(f"❌ Ошибка сохранения метрик: {e}")
            return False


def optimize_allan_for_training():
    """Быстрая функция оптимизации для обучения Allan"""
    optimizer = AllanPerformanceOptimizer()
    return optimizer.optimize_for_training()


def monitor_allan_training(duration_minutes: int = 60):
    """Быстрая функция мониторинга обучения Allan"""
    optimizer = AllanPerformanceOptimizer()
    optimizer.monitor_training(duration_minutes)
    return optimizer.generate_performance_report()


def cleanup_allan_resources():
    """Быстрая очистка ресурсов Allan"""
    optimizer = AllanPerformanceOptimizer()
    memory_results = optimizer.optimize_memory(aggressive=True)
    disk_results = optimizer.optimize_disk_space()
    return {**memory_results, **disk_results}


if __name__ == "__main__":
    # Демонстрация оптимизатора производительности
    optimizer = AllanPerformanceOptimizer()
    
    print("🔥 Allan Performance Optimizer - Демонстрация")
    print("=" * 60)
    
    # Показ текущего состояния
    optimizer.print_current_status()
    
    # Демо оптимизации
    print("\n🚀 Демонстрация оптимизации для обучения:")
    optimizer.optimize_for_training()