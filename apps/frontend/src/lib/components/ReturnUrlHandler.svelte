<!--
@component

Handles redirect to a stored return URL after login. When a user is redirected
to login from a protected route, the original URL is stored in sessionStorage.
This component checks for that URL and redirects back to it.

Include this component once in the authenticated layout.
-->
<script lang="ts">
  import { goto } from '$app/navigation';

  const returnUrl = sessionStorage.getItem('returnUrl');

  if (returnUrl && returnUrl !== window.location.pathname + window.location.search) {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- dynamic return URL from sessionStorage
    goto(returnUrl)
      .then(() => {
        sessionStorage.removeItem('returnUrl');
      })
      .catch((err) => {
        console.warn('Return URL navigation failed, clearing:', err);
        sessionStorage.removeItem('returnUrl');
      });
  } else {
    sessionStorage.removeItem('returnUrl');
  }
</script>
