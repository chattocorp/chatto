// Verify ownership and cleanup of the packaged hotkey without posting keyboard input.
import Carbon
import Darwin
import Foundation

let helper = URL(fileURLWithPath: CommandLine.arguments[1])

func launch(trigger: String = "F8") throws -> (Process, Pipe, Pipe) {
    let child = Process()
    let input = Pipe()
    let output = Pipe()
    child.executableURL = helper
    child.arguments = ["15", trigger]
    child.standardInput = input
    child.standardOutput = output
    try child.run()
    return (child, input, output)
}

func checkRegistration(trigger: String = "F8", checkConflict: Bool) throws {
    let (listener, input, output) = try launch(trigger: trigger)
    defer { if listener.isRunning { listener.terminate() } }
    // A pipe read may return only part of a line.
    var ready = Data()
    while ready.last != 10 {
        let part = output.fileHandleForReading.availableData
        if part.isEmpty {
            listener.waitUntilExit()
            fatalError("Registration of \(trigger) exited with status \(listener.terminationStatus)")
        }
        ready.append(part)
    }
    precondition(String(decoding: ready, as: UTF8.self) == "ready\n", "Unexpected response for \(trigger)")
    if trigger == "@," {
        // The packaged helper must reserve the physical Comma key for this binding.
        var other: EventHotKeyRef?
        let result = RegisterEventHotKey(UInt32(kVK_ANSI_Comma),
            UInt32(controlKey | cmdKey | optionKey | shiftKey),
            EventHotKeyID(signature: 1, id: 1), GetApplicationEventTarget(),
            UInt32(kEventHotKeyExclusive), &other)
        precondition(result == OSStatus(eventHotKeyExistsErr))
    }
    if checkConflict {
        let (duplicate, duplicateInput, _) = try launch()
        withExtendedLifetime(duplicateInput) { duplicate.waitUntilExit() }
        precondition(duplicate.terminationStatus != 0)
    }
    try input.fileHandleForWriting.close()
    listener.waitUntilExit()
    precondition(listener.terminationStatus == 0)
}

alarm(15)
try checkRegistration(checkConflict: true)
try checkRegistration(checkConflict: false)
try checkRegistration(trigger: "@,", checkConflict: false)

// Use installed layout data without switching the user's active keyboard.
func checkLayout(_ layoutID: String, _ mask: Int32, _ trigger: String, _ expected: Int) {
    let filter = [kTISPropertyInputSourceID as String: "com.apple.keylayout." + layoutID] as CFDictionary
    let sources = TISCreateInputSourceList(filter, true).takeRetainedValue() as! [TISInputSource]
    precondition(!sources.isEmpty, "Missing layout: \(layoutID)")
    let actual = printable_key_code(sources[0], mask, trigger)
    precondition(actual == expected, "\(layoutID) \(trigger): expected \(expected), got \(actual)")
}
checkLayout("French", 1, "1", kVK_ANSI_1)          // Control + physical Digit1 (unshifted &)
checkLayout("German", 9, "/", kVK_ANSI_7)          // Control + Shift + 7 produces /
checkLayout("French", 1, "A", kVK_ANSI_Q)          // Layout letter takes priority over position
checkLayout("US", 5, "M", kVK_ANSI_M)             // Option produces a non-shortcut character
checkLayout("French", 1, "@,", kVK_ANSI_Comma)    // Physical Comma produces a semicolon
checkLayout("French", 1, ",", kVK_ANSI_M)         // Literal comma uses a different position
