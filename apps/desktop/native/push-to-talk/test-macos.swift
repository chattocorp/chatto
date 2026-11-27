// Verify ownership and cleanup of the packaged hotkey without posting keyboard input.
import Darwin
import Foundation

let helper = URL(fileURLWithPath: CommandLine.arguments[1])

func launch() throws -> (Process, Pipe, Pipe) {
    let child = Process()
    let input = Pipe()
    let output = Pipe()
    child.executableURL = helper
    child.arguments = ["15", "F8"]
    child.standardInput = input
    child.standardOutput = output
    try child.run()
    return (child, input, output)
}

func checkRegistration(checkConflict: Bool) throws {
    let (listener, input, output) = try launch()
    defer { if listener.isRunning { listener.terminate() } }
    let ready = output.fileHandleForReading.availableData
    precondition(String(decoding: ready, as: UTF8.self) == "ready\n")
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
