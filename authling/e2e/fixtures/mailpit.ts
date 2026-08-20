import type { APIRequestContext } from '@playwright/test';

interface MessageSummary {
  ID: string;
}

interface MessagesResponse {
  messages: MessageSummary[];
}

interface Message {
  Subject: string;
  Text: string;
}

export async function messageCount(
  request: APIRequestContext,
  mailpitURL: string
): Promise<number> {
  const response = await request.get(`${mailpitURL}/api/v1/messages`);
  if (!response.ok()) {
    throw new Error(`Mailpit message list returned ${response.status()}`);
  }
  const mailbox = (await response.json()) as MessagesResponse;
  return mailbox.messages.length;
}

export async function waitForVerificationCode(
  request: APIRequestContext,
  mailpitURL: string,
  timeoutMs = 10_000
): Promise<string> {
  return waitForCode(
    request,
    mailpitURL,
    'Your Authling verification code',
    /verification code is ([0-9]{6})\./,
    timeoutMs
  );
}

export async function waitForPasswordResetCode(
  request: APIRequestContext,
  mailpitURL: string,
  timeoutMs = 10_000
): Promise<string> {
  return waitForCode(
    request,
    mailpitURL,
    'Your Authling password reset code',
    /password reset code is ([0-9]{6})\./,
    timeoutMs
  );
}

async function waitForCode(
  request: APIRequestContext,
  mailpitURL: string,
  subject: string,
  pattern: RegExp,
  timeoutMs: number
): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const response = await request.get(`${mailpitURL}/api/v1/messages`);
    if (response.ok()) {
      const mailbox = (await response.json()) as MessagesResponse;
      const latest = mailbox.messages[0];
      if (latest) {
        const messageResponse = await request.get(`${mailpitURL}/api/v1/message/${latest.ID}`);
        if (messageResponse.ok()) {
          const message = (await messageResponse.json()) as Message;
          if (message.Subject === subject) {
            const match = pattern.exec(message.Text);
            if (match) return match[1];
          }
        }
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error(`Mailpit did not receive ${subject} within ${timeoutMs}ms`);
}
