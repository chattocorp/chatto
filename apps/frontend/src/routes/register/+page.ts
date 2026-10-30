import { redirect } from '@sveltejs/kit';
import { getRegistrationAdmission } from '$lib/auth/registrationAdmission';
import { createExternalIdentityFlowAPI } from '$lib/api/externalIdentities';
import type { PageLoad } from './$types';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';

export const load = (async ({ parent, url, fetch }) => {
  const { user, serverInfo } = await parent();
  const raw = url.searchParams.get('redirect') ?? '';
  const redirectUrl = isSafeInternalPath(raw) ? raw : '/';
  if (user) redirect(302, redirectUrl === '/' ? '/chat' : redirectUrl);
  const externalIdentityToken = url.searchParams.get('external_identity') ?? '';
  let invitationRequired = serverInfo?.accountCreationPolicy === 'invite_only';
  if (externalIdentityToken) {
    const pending = await createExternalIdentityFlowAPI()
      .getPending(externalIdentityToken)
      .catch(() => null);
    if (!pending?.invitationRequired)
      redirect(302, `/sso/confirm?token=${encodeURIComponent(externalIdentityToken)}`);
    // The pending flow has a fresher admission decision than public discovery.
    invitationRequired = true;
  }
  return {
    redirectUrl,
    invitationRequired,
    admissionId:
      invitationRequired && !externalIdentityToken ? await getRegistrationAdmission(fetch) : '',
    externalIdentityToken
  };
}) satisfies PageLoad;
