export type ServerProfile = {
  name: string;
  version: string;
  logoUrl: string | null;
  bannerUrl: string | null;
  welcomeMessage: string | null;
  description: string | null;
  motd: string | null;
};

type ServerProfileLike =
  | {
      name?: string;
      version?: string;
      logoUrl?: string;
      bannerUrl?: string;
      welcomeMessage?: string;
      description?: string;
      motd?: string;
      publicProfile?: {
        name?: string;
        version?: string;
        logoUrl?: string;
        bannerUrl?: string;
        welcomeMessage?: string;
        description?: string;
      };
    }
  | null
  | undefined;

export function mapServerProfile(profile: ServerProfileLike): ServerProfile {
  const publicProfile = profile?.publicProfile ?? profile;
  return {
    name: publicProfile?.name || "Chatto",
    version: publicProfile?.version || "",
    logoUrl: publicProfile?.logoUrl ?? null,
    bannerUrl: publicProfile?.bannerUrl ?? null,
    welcomeMessage: publicProfile?.welcomeMessage ?? null,
    description: publicProfile?.description ?? null,
    motd: profile?.motd ?? null,
  };
}
