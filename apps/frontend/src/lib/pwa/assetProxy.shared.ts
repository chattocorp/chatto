export const ASSET_PROXY_PATH_PREFIX = '/__chatto/assets/';

export type AssetProxyServer = {
  id: string;
  url: string;
};

export type AssetProxyTarget = {
  serverId: string;
  virtualPath: string;
  targetUrl: string;
};
