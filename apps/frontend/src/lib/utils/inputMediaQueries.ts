/** Mobile frames and task sheets require narrow touch windows. Keep the
 * matching mobile-presentation variant in app.css in sync. */
export const NARROW_TOUCH_QUERY = '(width < 768px) and (any-pointer: coarse)';

/** Touch-only devices have a coarse pointer and no hover input, such as phones
 * and tablets without a mouse or trackpad. Devices without any pointer input,
 * such as headless browsers, do not match. */
export const TOUCH_ONLY_QUERY = '(any-pointer: coarse) and (any-hover: none)';
