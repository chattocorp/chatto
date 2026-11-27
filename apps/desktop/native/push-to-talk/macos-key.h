#pragma once
#include <Carbon/Carbon.h>

// Match the recorder: use the character produced with Shift/Option, or the
// physical Key, Digit, Comma or Slash position for other text and dead keys.
static inline int printable_key_code(TISInputSourceRef source, int mask, const char *name) {
    static const char physical[] = {
        [kVK_ANSI_A] = 'A', [kVK_ANSI_B] = 'B', [kVK_ANSI_C] = 'C', [kVK_ANSI_D] = 'D',
        [kVK_ANSI_E] = 'E', [kVK_ANSI_F] = 'F', [kVK_ANSI_G] = 'G', [kVK_ANSI_H] = 'H',
        [kVK_ANSI_I] = 'I', [kVK_ANSI_J] = 'J', [kVK_ANSI_K] = 'K', [kVK_ANSI_L] = 'L',
        [kVK_ANSI_M] = 'M', [kVK_ANSI_N] = 'N', [kVK_ANSI_O] = 'O', [kVK_ANSI_P] = 'P',
        [kVK_ANSI_Q] = 'Q', [kVK_ANSI_R] = 'R', [kVK_ANSI_S] = 'S', [kVK_ANSI_T] = 'T',
        [kVK_ANSI_U] = 'U', [kVK_ANSI_V] = 'V', [kVK_ANSI_W] = 'W', [kVK_ANSI_X] = 'X',
        [kVK_ANSI_Y] = 'Y', [kVK_ANSI_Z] = 'Z', [kVK_ANSI_0] = '0', [kVK_ANSI_1] = '1',
        [kVK_ANSI_2] = '2', [kVK_ANSI_3] = '3', [kVK_ANSI_4] = '4', [kVK_ANSI_5] = '5',
        [kVK_ANSI_6] = '6', [kVK_ANSI_7] = '7', [kVK_ANSI_8] = '8', [kVK_ANSI_9] = '9',
        [kVK_ANSI_Comma] = ',', [kVK_ANSI_Slash] = '/'
    };
    if (name[0] == '@' && name[1] && !name[2]) {
        for (int code = 0; code < (int)sizeof(physical); code++)
            if (physical[code] == name[1])
                return code;
        return -1;
    }
    if (!name[0] || name[1])
        return -1;
    char trigger = name[0];
    CFDataRef data = TISGetInputSourceProperty(source, kTISPropertyUnicodeKeyLayoutData);
    if (!data)
        return -1;
    const UCKeyboardLayout *layout = (const UCKeyboardLayout *)CFDataGetBytePtr(data);
    UInt32 modifiers = (((mask & 8) ? shiftKey : 0) | ((mask & 4) ? optionKey : 0)) >> 8;
    int fallback = -1;
    // Search the typing area first. Keypad digits can otherwise shadow the number row.
    for (int code = 0; code < 128; code++) {
        UInt32 dead = 0;
        UniChar chars[4];
        UniCharCount length = 0;
        if (UCKeyTranslate(layout, code, kUCKeyActionDown, modifiers, LMGetKbdType(), 0,
                           &dead, 4, &length, chars) != noErr)
            continue;
        UniChar ch = length == 1 && !dead ? chars[0] : 0;
        if (ch >= 'a' && ch <= 'z')
            ch -= 'a' - 'A';
        if (ch == trigger) {
            if (code <= kVK_Space)
                return code;
            if (fallback == -1)
                fallback = code;
        } else if (code < (int)sizeof(physical) && physical[code] == trigger &&
                   !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') &&
                   ch != ',' && ch != '/') {
            fallback = code;
        }
    }
    return fallback;
}
