// Register one Carbon hotkey. The OS delivers both edges without input monitoring.
#import <AppKit/AppKit.h>
#include "macos-key.h"
#include <dispatch/dispatch.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static OSStatus edge(EventHandlerCallRef next, EventRef event, void *context) {
    puts(GetEventKind(event) == kEventHotKeyPressed ? "down" : "up");
    return noErr;
}

static int key_code(const char *name, int mask) {
    const char *names[] = {"Space", "Enter", "Left", "Right", "Up", "Down", "F1",  "F2",  "F3",
                           "F4",    "F5",    "F6",   "F7",    "F8", "F9",   "F10", "F11", "F12"};
    const int codes[] = {49,  36, 123, 124, 126, 125, 122, 120, 99,
                         118, 96, 97,  98,  100, 101, 109, 103, 111};
    for (size_t i = 0; i < sizeof(codes) / sizeof(*codes); i++)
        if (!strcmp(name, names[i]))
            return codes[i];

    // A physical DOM position does not depend on the active input source.
    if (name[0] == '@')
        return printable_key_code(NULL, mask, name);
    // Resolve printable triggers on the current keyboard layout.
    TISInputSourceRef source = TISCopyCurrentASCIICapableKeyboardLayoutInputSource();
    if (!source)
        return -1;
    int match = printable_key_code(source, mask, name);
    CFRelease(source);
    return match;
}

int main(int argc, char **argv) {
    if (argc != 3)
        return 1;
    setbuf(stdout, NULL);
    NSApplication *app = NSApplication.sharedApplication;
    [app setActivationPolicy:NSApplicationActivationPolicyProhibited];
    int mask = atoi(argv[1]), key = key_code(argv[2], mask);
    UInt32 mods = ((mask & 1) ? controlKey : 0) | ((mask & 2) ? cmdKey : 0) |
                  ((mask & 4) ? optionKey : 0) | ((mask & 8) ? shiftKey : 0);
    EventTypeSpec events[] = {{kEventClassKeyboard, kEventHotKeyPressed},
                              {kEventClassKeyboard, kEventHotKeyReleased}};
    EventHotKeyRef ref;
    EventHotKeyID id = {'CPTT', 1};
    if (key < 0 || InstallApplicationEventHandler(edge, 2, events, NULL, NULL) ||
        RegisterEventHotKey(key, mods, id, GetApplicationEventTarget(), kEventHotKeyExclusive, &ref))
        return 1;
    dispatch_source_t parent = dispatch_source_create(DISPATCH_SOURCE_TYPE_READ, STDIN_FILENO, 0,
                                                      dispatch_get_main_queue());
    dispatch_source_set_event_handler(parent, ^{
      exit(0);
    });
    dispatch_resume(parent);
    puts("ready");
    [app run];
    return 0;
}
