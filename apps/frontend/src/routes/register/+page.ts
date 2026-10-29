import { redirect } from '@sveltejs/kit';
import { getRegistrationAdmission } from '$lib/auth/registrationAdmission';
import { createExternalIdentityFlowAPI } from '$lib/api/externalIdentities';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url, fetch }) => {
  const { user, serverInfo } = await parent();
  if (user) redirect(302, '/chat');
  const externalIdentityToken = url.searchParams.get('external_identity') ?? '';
  if (externalIdentityToken) {
    const pending = await createExternalIdentityFlowAPI()
      .getPending(externalIdentityToken)
      .catch(() => null);
    if (!pending?.invitationRequired)
      redirect(302, `/sso/confirm?token=${encodeURIComponent(externalIdentityToken)}`);
  }
  return {
    admissionId:
      serverInfo?.accountCreationPolicy === 'invite_only' && !externalIdentityToken
        ? await getRegistrationAdmission(fetch)
        : '',
    externalIdentityToken
  };
};
