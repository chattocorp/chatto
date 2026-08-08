export type ProtobufDuration = {
	seconds: bigint;
	nanos: number;
};

export function protobufDurationToSeconds(
	duration: ProtobufDuration | null | undefined
): number | undefined {
	if (duration == null) return undefined;
	return Number(duration.seconds) + duration.nanos / 1_000_000_000;
}

export function secondsToProtobufDuration(seconds: number): ProtobufDuration {
	const wholeSeconds = Math.trunc(seconds);
	const nanos = Math.round((seconds - wholeSeconds) * 1_000_000_000);
	return { seconds: BigInt(wholeSeconds), nanos };
}
