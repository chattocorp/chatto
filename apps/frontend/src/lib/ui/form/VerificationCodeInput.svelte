<!--
@component

A six-digit verification-code field. It supports typing, pasting a complete
code, keyboard backspace navigation, and one-time-code autofill.
-->
<script lang="ts">
	let {
		value = $bindable(''),
		disabled = false,
		autofocus = false,
		label,
		digitLabel
	}: {
		value?: string;
		disabled?: boolean;
		autofocus?: boolean;
		label: string;
		digitLabel: (number: number) => string;
	} = $props();

	let inputs: Array<HTMLInputElement | undefined> = [];
	let digits = $state(codeDigits(value));
	let publishedValue = value;

	function codeDigits(code: string): string[] {
		return [...code.replace(/\D/g, '').slice(0, 6), '', '', '', '', '', ''].slice(0, 6);
	}

	function currentDigits(): string[] {
		return value === publishedValue ? digits : codeDigits(value);
	}

	function publishDigits(next: string[]) {
		digits = next;
		publishedValue = next.join('');
		value = publishedValue;
	}

	function registerInput(index: number) {
		return (input: HTMLInputElement) => {
			inputs[index] = input;
			return () => {
				if (inputs[index] === input) inputs[index] = undefined;
			};
		};
	}

	function focusInput(input: HTMLInputElement) {
		queueMicrotask(() => input.focus());
	}

	function applyCodeFrom(index: number, inputValue: string) {
		const next = [...currentDigits()];
		const insertedDigits = inputValue
			.replace(/\D/g, '')
			.slice(0, 6 - index)
			.split('');
		if (insertedDigits.length === 0) {
			next[index] = '';
			publishDigits(next);
			return;
		}
		for (const [offset, digit] of insertedDigits.entries()) next[index + offset] = digit;
		publishDigits(next);
		inputs[Math.min(index + insertedDigits.length, 5)]?.focus();
	}

	function handleInput(index: number, event: Event) {
		applyCodeFrom(index, (event.currentTarget as HTMLInputElement).value);
	}

	function handlePaste(index: number, event: ClipboardEvent) {
		event.preventDefault();
		applyCodeFrom(index, event.clipboardData?.getData('text') ?? '');
	}

	function handleKeydown(index: number, event: KeyboardEvent) {
		const current = currentDigits();
		if (event.key === 'Backspace' && !current[index] && index > 0) {
			event.preventDefault();
			current[index - 1] = '';
			publishDigits(current);
			inputs[index - 1]?.focus();
		}
	}

	export function focus() {
		queueMicrotask(() => inputs[0]?.focus());
	}
</script>

<div class="grid grid-cols-6 gap-2" aria-label={label}>
	{#each Array(6) as _, index (index)}
		<input
			{@attach registerInput(index)}
			{@attach autofocus && index === 0 && focusInput}
			value={currentDigits()[index]}
			type="text"
			inputmode="numeric"
			pattern="[0-9]*"
			maxlength="6"
			autocomplete={index === 0 ? 'one-time-code' : 'off'}
			aria-label={digitLabel(index + 1)}
			{disabled}
			oninput={(event) => handleInput(index, event)}
			onpaste={(event) => handlePaste(index, event)}
			onkeydown={(event) => handleKeydown(index, event)}
			class="h-14 rounded-lg border border-text/20 bg-input text-center text-xl font-semibold transition-[border-color,box-shadow] outline-none focus:border-action focus:ring-2 focus:ring-action/30 disabled:opacity-60"
		/>
	{/each}
</div>
