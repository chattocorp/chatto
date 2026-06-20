export type SidebarLinkTarget =
  | {
      valid: true;
      href: string;
      target?: '_blank';
      rel?: 'noopener noreferrer';
    }
  | {
      valid: false;
      href: '#';
    };

export type SidebarLinkAnchorAttributes = {
  href: string;
  target?: '_blank';
  rel?: 'noopener noreferrer';
};

function invalidSidebarLinkTarget(): SidebarLinkTarget {
  return { valid: false, href: '#' };
}

function parseServerBaseURL(serverBaseURL: string | null | undefined): URL | null {
  if (!serverBaseURL) return null;
  try {
    return new URL(serverBaseURL);
  } catch {
    return null;
  }
}

export function sidebarLinkTarget(
  rawURL: string,
  serverBaseURL: string | null | undefined
): SidebarLinkTarget {
  const value = rawURL.trim();
  if (!value) return invalidSidebarLinkTarget();

  const serverURL = parseServerBaseURL(serverBaseURL);

  if (value.startsWith('/')) {
    if (!serverURL || value.startsWith('//') || value.includes('\\')) {
      return invalidSidebarLinkTarget();
    }
    try {
      return { valid: true, href: new URL(value, serverURL).toString() };
    } catch {
      return invalidSidebarLinkTarget();
    }
  }

  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return invalidSidebarLinkTarget();
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return invalidSidebarLinkTarget();
  }

  if (serverURL && parsed.host === serverURL.host) {
    return { valid: true, href: parsed.toString() };
  }

  return {
    valid: true,
    href: parsed.toString(),
    target: '_blank',
    rel: 'noopener noreferrer'
  };
}

export function sidebarLinkAnchorAttributes(
  target: SidebarLinkTarget
): SidebarLinkAnchorAttributes {
  if (!target.valid) return { href: target.href };
  return { href: target.href, target: target.target, rel: target.rel };
}
