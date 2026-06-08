<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { clearCachedUser } from '$lib/auth/loadAuth';
	import AuthLayout from '$lib/components/AuthLayout.svelte';
	import { serverRegistry } from '$lib/state/server/registry.svelte';
	import { Divider } from '$lib/ui';
	import PageTitle from '$lib/ui/PageTitle.svelte';
	import { Button, FormError, TextInput, validate, z } from '$lib/ui/form';

	const { data } = $props();

	// Redirect if already logged in (use layout data, consistent with /login)
	// svelte-ignore state_referenced_locally
	if (data.user) {
		goto(resolve('/'));
	}

	type Step = 'email' | 'code' | 'details';

	const origin = $derived(serverRegistry.originServer);
	const originStore = $derived(origin ? serverRegistry.tryGetStore(origin.id) : undefined);
	const registrationEnabled = $derived(originStore?.serverInfo.directRegistrationEnabled ?? true);

	let step = $state<Step>('email');
	let email = $state('');
	let codeDigits = $state(['', '', '', '', '', '']);
	let completionToken = $state('');
	let login = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let error = $state('');
	let isLoading = $state(false);
	let isResending = $state(false);
	let codeInputs: HTMLInputElement[] = [];

	const emailSchema = z.string().email('Please enter a valid email address');
	const loginSchema = z
		.string()
		.min(2, 'Must be at least 2 characters')
		.max(32, 'Must be at most 32 characters')
		.regex(/^[a-zA-Z0-9._-]+$/, 'Only letters, numbers, dots, dashes, underscores')
		.refine((val) => !val.includes('..'), 'No consecutive periods allowed');
	const passwordSchema = z.string().min(8, 'Must be at least 8 characters');

	const normalizedEmail = $derived(email.trim().toLowerCase());
	const emailError = $derived(email ? validate(emailSchema, email) : undefined);
	const code = $derived(codeDigits.join(''));
	const codeComplete = $derived(code.length === 6);
	const loginError = $derived(login ? validate(loginSchema, login) : undefined);
	const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
	const confirmError = $derived(
		confirmPassword && password !== confirmPassword ? 'Passwords do not match' : undefined
	);
	const canSubmitEmail = $derived(normalizedEmail && !emailError);
	const canSubmitDetails = $derived(
		completionToken && login && password && confirmPassword && !loginError && !passwordError && !confirmError
	);

	async function requestRegistrationCode(options: { resend?: boolean } = {}) {
		error = '';
		if (emailError || !normalizedEmail) {
			error = emailError || 'Please enter a valid email address';
			return;
		}

		if (options.resend) {
			isResending = true;
		} else {
			isLoading = true;
		}

		try {
			const response = await fetch('/auth/register', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email: normalizedEmail })
			});
			const body = await response.json();

			if (!response.ok) {
				error = body.error || 'Registration failed';
				return;
			}

			codeDigits = ['', '', '', '', '', ''];
			completionToken = '';
			step = 'code';
			queueMicrotask(() => codeInputs[0]?.focus());
		} catch (err) {
			error = err instanceof Error ? err.message : 'Registration failed';
		} finally {
			isLoading = false;
			isResending = false;
		}
	}

	async function handleEmailSubmit(e: Event) {
		e.preventDefault();
		await requestRegistrationCode();
	}

	function applyCodeFrom(index: number, value: string) {
		const digits = value.replace(/\D/g, '').slice(0, 6 - index).split('');
		if (digits.length === 0) {
			codeDigits[index] = '';
			return;
		}
		for (const [offset, digit] of digits.entries()) {
			codeDigits[index + offset] = digit;
		}
		const nextIndex = Math.min(index + digits.length, codeDigits.length - 1);
		codeInputs[nextIndex]?.focus();
	}

	function handleCodeInput(index: number, e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		applyCodeFrom(index, input.value);
	}

	function handleCodePaste(index: number, e: ClipboardEvent) {
		e.preventDefault();
		applyCodeFrom(index, e.clipboardData?.getData('text') ?? '');
	}

	function handleCodeKeydown(index: number, e: KeyboardEvent) {
		if (e.key === 'Backspace' && codeDigits[index] === '' && index > 0) {
			e.preventDefault();
			codeDigits[index - 1] = '';
			codeInputs[index - 1]?.focus();
		}
	}

	async function handleCodeSubmit(e: Event) {
		e.preventDefault();
		if (!codeComplete) {
			error = 'Enter the 6-digit code from your email';
			return;
		}

		error = '';
		isLoading = true;
		try {
			const response = await fetch('/auth/register/verify-code', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email: normalizedEmail, code })
			});
			const body = await response.json();

			if (!response.ok) {
				error = body.error || 'Invalid or expired registration code';
				return;
			}
			completionToken = body.completionToken;
			step = 'details';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Registration failed';
		} finally {
			isLoading = false;
		}
	}

	async function handleDetailsSubmit(e: Event) {
		e.preventDefault();
		if (!completionToken || loginError || passwordError || confirmError) {
			error = loginError || passwordError || confirmError || 'Please fix the errors above';
			return;
		}

		error = '';
		isLoading = true;
		try {
			const response = await fetch('/auth/register/complete', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					token: completionToken,
					login,
					password,
					passwordConfirmation: confirmPassword
				}),
				credentials: 'include'
			});
			const body = await response.json();

			if (!response.ok) {
				error = body.error || 'Registration failed';
				return;
			}

			clearCachedUser();
			await invalidateAll();

			const returnUrl = sessionStorage.getItem('returnUrl');
			if (returnUrl) {
				sessionStorage.removeItem('returnUrl');
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- dynamic return URL from sessionStorage
				goto(returnUrl);
			} else {
				goto(resolve('/'), { replaceState: true });
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Registration failed';
		} finally {
			isLoading = false;
		}
	}
</script>

<PageTitle title="Create Account" />

<AuthLayout>
	<h1 class="mb-6 text-center text-2xl font-bold">
		{step === 'code' ? 'Check your email' : step === 'details' ? 'Complete Registration' : 'Create Account'}
	</h1>

	{#if !registrationEnabled}
		<p class="text-center text-muted">Registration is not available on this instance.</p>
	{:else if step === 'email'}
		<form onsubmit={handleEmailSubmit} class="flex flex-col gap-4">
			<TextInput
				id="email"
				label="Email"
				type="email"
				bind:value={email}
				placeholder="you@example.com"
				disabled={isLoading}
				required
				autofocus
				autocomplete="email"
				error={emailError}
			/>

			<FormError {error} />

			<Button type="submit" size="lg" disabled={!canSubmitEmail} loading={isLoading} loadingText="Sending...">
				Continue
				<span class="iconify uil--arrow-right"></span>
			</Button>
		</form>
	{:else if step === 'code'}
		<form onsubmit={handleCodeSubmit} class="flex flex-col gap-5">
			<div class="text-center">
				<p class="text-muted">Enter the verification code sent to</p>
				<p class="mt-1 break-words font-semibold">{normalizedEmail}</p>
			</div>

			<div class="grid grid-cols-6 gap-2" aria-label="Verification code">
				{#each codeDigits as digit, index (index)}
					<input
						bind:this={codeInputs[index]}
						value={digit}
						type="text"
						inputmode="numeric"
						pattern="[0-9]*"
						maxlength="6"
						autocomplete={index === 0 ? 'one-time-code' : 'off'}
						aria-label={`Digit ${index + 1}`}
						disabled={isLoading}
						oninput={(e) => handleCodeInput(index, e)}
						onpaste={(e) => handleCodePaste(index, e)}
						onkeydown={(e) => handleCodeKeydown(index, e)}
						class="h-14 rounded-lg border border-text/20 bg-input text-center text-xl font-semibold outline-none transition focus:border-accent focus:ring-2 focus:ring-accent/30 disabled:opacity-60"
					/>
				{/each}
			</div>

			<div class="text-center text-sm text-muted">
				Didn't receive the code?
				<button
					type="button"
					class="link cursor-pointer disabled:cursor-default disabled:opacity-60"
					disabled={isLoading || isResending}
					onclick={() => requestRegistrationCode({ resend: true })}
				>
					{isResending ? 'Resending...' : 'Resend'}
				</button>
			</div>

			<FormError {error} />

			<Button type="submit" size="lg" disabled={!codeComplete} loading={isLoading} loadingText="Checking...">
				Submit
			</Button>
		</form>
	{:else}
		<form onsubmit={handleDetailsSubmit} class="flex flex-col gap-4">
			<TextInput
				id="login"
				label="Username"
				bind:value={login}
				placeholder="your_username"
				disabled={isLoading}
				required
				autocomplete="username"
				error={loginError}
			/>

			<TextInput
				id="password"
				label="Password"
				type="password"
				bind:value={password}
				placeholder="At least 8 characters"
				disabled={isLoading}
				required
				minlength={8}
				autocomplete="new-password"
				error={passwordError}
			/>

			<TextInput
				id="confirmPassword"
				label="Confirm Password"
				type="password"
				bind:value={confirmPassword}
				placeholder="Enter password again"
				disabled={isLoading}
				required
				autocomplete="new-password"
				error={confirmError}
			/>

			<FormError {error} />

			<Button
				type="submit"
				size="lg"
				disabled={!canSubmitDetails}
				loading={isLoading}
				loadingText="Creating account..."
			>
				<span class="iconify uil--user-plus"></span>
				Create Account
			</Button>
		</form>
	{/if}

	<Divider label="or" />

	<a href={resolve('/login')} class="btn-secondary btn-lg block w-full text-center">
		Sign In
	</a>
</AuthLayout>
