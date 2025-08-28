#!/usr/bin/env python3
"""
Пример использования Allan Dataset Downloader
Демонстрация загрузки и предобработки датасетов
"""

from allan_dataset_downloader import AllanDatasetDownloader, quick_download_dataset

def main():
    """Основная функция демонстрации"""
    print("🚀 Allan Dataset Downloader - Пример использования")
    print("=" * 60)
    
    # Создаем экземпляр загрузчика
    downloader = AllanDatasetDownloader()
    
    # Показываем доступные датасеты
    print("\n📚 Доступные датасеты:")
    downloader.list_available_datasets()
    
    # Показываем текущее использование диска
    print("\n💾 Текущее использование диска:")
    disk_usage = downloader.get_disk_usage()
    for key, value in disk_usage.items():
        if "percent" in key:
            print(f"  {key}: {value:.1f}%")
        else:
            print(f"  {key}: {value:.2f} ГБ")
    
    # Пример 1: Загрузка небольшого датасета с предобработкой
    print("\n" + "="*60)
    print("📥 Пример 1: Загрузка датасета 'rucola' с предобработкой")
    
    success = downloader.download_and_preprocess("rucola")
    if success:
        print("✅ Датасет 'rucola' успешно загружен и обработан!")
        
        # Показываем статус
        status = downloader.get_dataset_status("rucola")
        print(f"📊 Статус датасета:")
        print(f"  Сырые данные: {'✅' if status['raw_exists'] else '❌'}")
        print(f"  Обработанные: {'✅' if status['processed_exists'] else '❌'}")
        print(f"  Кэш: {'✅' if status['cached_exists'] else '❌'}")
    else:
        print("❌ Ошибка загрузки датасета 'rucola'")
    
    # Пример 2: Загрузка только сырых данных без предобработки
    print("\n" + "="*60)
    print("📥 Пример 2: Загрузка только сырых данных 'russian_paraphrase'")
    
    success = downloader.download_and_preprocess("russian_paraphrase", skip_preprocessing=True)
    if success:
        print("✅ Сырые данные 'russian_paraphrase' загружены!")
        
        # Показываем статус
        status = downloader.get_dataset_status("russian_paraphrase")
        print(f"📊 Статус датасета:")
        print(f"  Сырые данные: {'✅' if status['raw_exists'] else '❌'}")
        print(f"  Обработанные: {'✅' if status['processed_exists'] else '❌'}")
        print(f"  Кэш: {'✅' if status['cached_exists'] else '❌'}")
    else:
        print("❌ Ошибка загрузки сырых данных 'russian_paraphrase'")
    
    # Пример 3: Использование быстрых функций
    print("\n" + "="*60)
    print("⚡ Пример 3: Использование быстрых функций")
    
    # Быстрая загрузка
    print("🚀 Быстрая загрузка датасета 'sberquad'...")
    success = quick_download_dataset("sberquad")
    if success:
        print("✅ Быстрая загрузка завершена!")
    else:
        print("❌ Быстрая загрузка не удалась")
    
    # Показываем финальное использование диска
    print("\n💾 Финальное использование диска:")
    disk_usage = downloader.get_disk_usage()
    for key, value in disk_usage.items():
        if "percent" in key:
            print(f"  {key}: {value:.1f}%")
        else:
            print(f"  {key}: {value:.2f} ГБ")
    
    # Очистка временных файлов
    print("\n🧹 Очистка временных файлов...")
    downloader.cleanup_temp_files()
    
    print("\n🎉 Демонстрация завершена!")
    print("📁 Все датасеты сохранены на Google Drive")
    print("🔧 Используйте AllanDatasetDownloader для работы с датасетами")

if __name__ == "__main__":
    main()
