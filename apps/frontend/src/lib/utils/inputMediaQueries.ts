/** Mobile frames and task sheets require narrow touch windows. Keep the
 * matching mobile-presentation variant in app.css in sync. */
export const NARROW_TOUCH_QUERY = '(width < 768px) and (any-pointer: coarse)';

/** Hover-revealed controls require a hovering fine pointer, such as a mouse or
 * trackpad. Touch-only devices do not match. Keep the matching hover-actions
 * variant in app.css in sync. */
export const HOVER_POINTER_QUERY = '(any-hover: hover) and (any-pointer: fine)';
