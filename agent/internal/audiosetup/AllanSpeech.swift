// AllanSpeech: on-device speech recognition for Allan on macOS.
//
//   AllanSpeech                         ask for the permission (first run)
//   AllanSpeech <audio> <out.txt> [ru]  transcribe; the text goes to out.txt,
//                                       an error to out.txt.err
//
// macOS allows speech recognition only for an app that explains why in its
// Info.plist, so this is a tiny app bundle and not a plain binary. Allan
// starts it with `open`, which makes the app its own responsible process.
import Foundation
import Speech

let args = CommandLine.arguments

func fail(_ out: String, _ message: String) -> Never {
    try? message.write(toFile: out + ".err", atomically: true, encoding: .utf8)
    exit(1)
}

func localeID(_ code: String) -> String {
    switch code.lowercased() {
    case "ru": return "ru-RU"
    case "en": return "en-US"
    case "de": return "de-DE"
    case "": return Locale.current.identifier
    default: return code
    }
}

if args.count < 3 {
    SFSpeechRecognizer.requestAuthorization { status in
        print(status == .authorized ? "authorized" : "denied")
        exit(status == .authorized ? 0 : 1)
    }
    RunLoop.main.run()
}

let input = args[1], out = args[2]
let lang = args.count > 3 ? args[3] : ""

SFSpeechRecognizer.requestAuthorization { status in
    guard status == .authorized else { fail(out, "нет разрешения на распознавание речи: Системные настройки → Конфиденциальность → Распознавание речи → AllanSpeech") }
    guard let recognizer = SFSpeechRecognizer(locale: Locale(identifier: localeID(lang))), recognizer.isAvailable else {
        fail(out, "распознавание для языка \(localeID(lang)) недоступно")
    }
    let request = SFSpeechURLRecognitionRequest(url: URL(fileURLWithPath: input))
    request.shouldReportPartialResults = false
    request.requiresOnDeviceRecognition = recognizer.supportsOnDeviceRecognition
    recognizer.recognitionTask(with: request) { result, error in
        if let error = error { fail(out, error.localizedDescription) }
        if let result = result, result.isFinal {
            try? result.bestTranscription.formattedString.write(toFile: out, atomically: true, encoding: .utf8)
            exit(0)
        }
    }
}
RunLoop.main.run()
