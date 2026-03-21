#!/usr/bin/env node

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { ImapFlow } from "imapflow";
import { z } from "zod";

// ---------------------------------------------------------------------------
// Config types
// ---------------------------------------------------------------------------

interface MailboxConfig {
  name: string;
  email: string;
  password: string;
  host?: string;
  port?: number;
}

interface GmailConfig {
  mailboxes: MailboxConfig[];
}

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

function loadConfig(): GmailConfig {
  const raw = process.env.GMAIL_MCP_CONFIG;
  if (!raw) {
    throw new Error(
      "GMAIL_MCP_CONFIG environment variable is not set. " +
        'Expected JSON: {"mailboxes":[{"name":"...","email":"...","password":"..."}]}'
    );
  }
  const parsed = JSON.parse(raw) as GmailConfig;
  if (!parsed.mailboxes || !Array.isArray(parsed.mailboxes) || parsed.mailboxes.length === 0) {
    throw new Error("GMAIL_MCP_CONFIG must contain a non-empty mailboxes array");
  }
  return parsed;
}

const config = loadConfig();

// ---------------------------------------------------------------------------
// IMAP helpers
// ---------------------------------------------------------------------------

function findMailbox(name: string): MailboxConfig {
  const mb = config.mailboxes.find((m) => m.name === name);
  if (!mb) {
    const available = config.mailboxes.map((m) => m.name).join(", ");
    throw new Error(`Mailbox "${name}" not found. Available: ${available}`);
  }
  return mb;
}

async function withImap<T>(mb: MailboxConfig, fn: (client: ImapFlow) => Promise<T>): Promise<T> {
  const client = new ImapFlow({
    host: mb.host ?? "imap.gmail.com",
    port: mb.port ?? 993,
    secure: true,
    auth: {
      user: mb.email,
      pass: mb.password,
    },
    logger: false as unknown as import("imapflow").Logger,
  });

  await client.connect();
  try {
    return await fn(client);
  } finally {
    await client.logout();
  }
}

// ---------------------------------------------------------------------------
// MCP Server
// ---------------------------------------------------------------------------

const server = new McpServer({
  name: "gmail-mcp",
  version: "1.0.0",
});

// Tool 1: gmail_list_mailboxes
server.tool("gmail_list_mailboxes", "List all configured Gmail mailboxes", {}, async () => {
  const mailboxes = config.mailboxes.map((m) => ({
    name: m.name,
    email: m.email,
    host: m.host ?? "imap.gmail.com",
  }));
  return { content: [{ type: "text" as const, text: JSON.stringify(mailboxes, null, 2) }] };
});

// Tool 2: gmail_list_folders
server.tool(
  "gmail_list_folders",
  "List all folders/labels in a Gmail mailbox with message counts",
  { mailbox: z.string().describe("Name of the configured mailbox") },
  async ({ mailbox }) => {
    const mb = findMailbox(mailbox);
    const folders = await withImap(mb, async (client) => {
      const result: Array<{ path: string; messages: number }> = [];
      const list = await client.list();
      for (const folder of list) {
        try {
          const status = await client.status(folder.path, { messages: true });
          result.push({ path: folder.path, messages: status.messages ?? 0 });
        } catch {
          result.push({ path: folder.path, messages: -1 });
        }
      }
      return result;
    });
    return { content: [{ type: "text" as const, text: JSON.stringify(folders, null, 2) }] };
  }
);

// Tool 3: gmail_search
server.tool(
  "gmail_search",
  "Search emails across one or all mailboxes by from, subject, date range, or body text",
  {
    mailbox: z.string().optional().describe("Mailbox name (omit to search all)"),
    from: z.string().optional().describe("Filter by sender address or name"),
    subject: z.string().optional().describe("Filter by subject text"),
    since: z.string().optional().describe("Messages since this ISO date (YYYY-MM-DD)"),
    before: z.string().optional().describe("Messages before this ISO date (YYYY-MM-DD)"),
    body: z.string().optional().describe("Search in message body text"),
    limit: z.number().optional().default(20).describe("Max results (default 20)"),
  },
  async ({ mailbox, from, subject, since, before, body, limit }) => {
    const mailboxes = mailbox ? [findMailbox(mailbox)] : config.mailboxes;
    const maxResults = limit ?? 20;
    const allResults: Array<Record<string, unknown>> = [];

    for (const mb of mailboxes) {
      if (allResults.length >= maxResults) break;
      try {
        const results = await withImap(mb, async (client) => {
          const lock = await client.getMailboxLock("INBOX");
          try {
            const searchCriteria: Record<string, unknown> = {};
            if (from) searchCriteria.from = from;
            if (subject) searchCriteria.subject = subject;
            if (since) searchCriteria.since = since;
            if (before) searchCriteria.before = before;
            if (body) searchCriteria.body = body;

            const searchResult = await client.search(searchCriteria, { uid: true });
            const uids = searchResult || [];
            if (!uids.length) return [];

            // Take only the last N UIDs (most recent)
            const selectedUids = uids.slice(-Math.min(maxResults - allResults.length, uids.length));
            const messages: Array<Record<string, unknown>> = [];

            for await (const msg of client.fetch(selectedUids, {
              uid: true,
              envelope: true,
            })) {
              messages.push({
                mailbox: mb.name,
                uid: msg.uid,
                from: msg.envelope?.from?.[0]?.address ?? "",
                subject: msg.envelope?.subject ?? "",
                date: msg.envelope?.date?.toISOString() ?? "",
              });
            }
            return messages;
          } finally {
            lock.release();
          }
        });
        allResults.push(...results);
      } catch (err) {
        allResults.push({
          mailbox: mb.name,
          error: err instanceof Error ? err.message : String(err),
        });
      }
    }

    return { content: [{ type: "text" as const, text: JSON.stringify(allResults, null, 2) }] };
  }
);

// Tool 4: gmail_read
server.tool(
  "gmail_read",
  "Read a full email by UID from a specific mailbox and folder",
  {
    mailbox: z.string().describe("Mailbox name"),
    folder: z.string().optional().default("INBOX").describe("Folder path (default INBOX)"),
    uid: z.number().describe("Email UID"),
  },
  async ({ mailbox, folder, uid }) => {
    const mb = findMailbox(mailbox);
    const email = await withImap(mb, async (client) => {
      const lock = await client.getMailboxLock(folder ?? "INBOX");
      try {
        const fetchResult = await client.fetchOne(uid, {
          uid: true,
          envelope: true,
          source: true,
        }, { uid: true });

        if (!fetchResult) {
          throw new Error(`Message with UID ${uid} not found`);
        }

        const envelope = fetchResult.envelope;
        const source = fetchResult.source?.toString("utf-8") ?? "";

        // Parse body from raw source — extract text/plain or text/html
        let bodyText = "";
        let bodyHtml = "";

        // Simple MIME parsing: look for text parts
        const parts = source.split(/\r?\n\r?\n/);
        if (parts.length > 1) {
          const bodyContent = parts.slice(1).join("\n\n");
          // Check for multipart
          const contentTypeMatch = source.match(/Content-Type:\s*([^\r\n;]+)/i);
          const contentType = contentTypeMatch?.[1]?.trim().toLowerCase() ?? "text/plain";

          if (contentType.startsWith("multipart/")) {
            const boundaryMatch = source.match(/boundary="?([^"\r\n]+)"?/i);
            if (boundaryMatch) {
              const boundary = boundaryMatch[1];
              const sections = bodyContent.split("--" + boundary);
              for (const section of sections) {
                const sectionTypeLower = section.toLowerCase();
                if (sectionTypeLower.includes("content-type: text/plain")) {
                  const sectionParts = section.split(/\r?\n\r?\n/);
                  if (sectionParts.length > 1) {
                    bodyText = sectionParts.slice(1).join("\n\n").trim();
                  }
                } else if (sectionTypeLower.includes("content-type: text/html")) {
                  const sectionParts = section.split(/\r?\n\r?\n/);
                  if (sectionParts.length > 1) {
                    bodyHtml = sectionParts.slice(1).join("\n\n").trim();
                  }
                }
              }
            }
          } else if (contentType === "text/html") {
            bodyHtml = bodyContent;
          } else {
            bodyText = bodyContent;
          }
        }

        return {
          from: envelope?.from?.[0]?.address ?? "",
          to: envelope?.to?.map((a: { address?: string }) => a.address).join(", ") ?? "",
          cc: envelope?.cc?.map((a: { address?: string }) => a.address).join(", ") ?? "",
          subject: envelope?.subject ?? "",
          date: envelope?.date?.toISOString() ?? "",
          body_text: bodyText || undefined,
          body_html: bodyHtml || undefined,
        };
      } finally {
        lock.release();
      }
    });

    return { content: [{ type: "text" as const, text: JSON.stringify(email, null, 2) }] };
  }
);

// Tool 5: gmail_list_recent
server.tool(
  "gmail_list_recent",
  "List the most recent N emails from a mailbox folder",
  {
    mailbox: z.string().describe("Mailbox name"),
    folder: z.string().optional().default("INBOX").describe("Folder path (default INBOX)"),
    limit: z.number().optional().default(10).describe("Number of recent emails (default 10)"),
  },
  async ({ mailbox, folder, limit }) => {
    const mb = findMailbox(mailbox);
    const maxResults = limit ?? 10;

    const messages = await withImap(mb, async (client) => {
      const lock = await client.getMailboxLock(folder ?? "INBOX");
      try {
        // Search for all messages, take the last N UIDs
        const searchResult = await client.search({ all: true }, { uid: true });
        const allUids = searchResult || [];
        const recentUids = allUids.slice(-maxResults);
        if (!recentUids.length) return [];

        const result: Array<Record<string, unknown>> = [];
        for await (const msg of client.fetch(recentUids, {
          uid: true,
          envelope: true,
          flags: true,
        })) {
          result.push({
            uid: msg.uid,
            from: msg.envelope?.from?.[0]?.address ?? "",
            subject: msg.envelope?.subject ?? "",
            date: msg.envelope?.date?.toISOString() ?? "",
            flags: Array.from(msg.flags ?? []),
          });
        }
        return result;
      } finally {
        lock.release();
      }
    });

    return { content: [{ type: "text" as const, text: JSON.stringify(messages, null, 2) }] };
  }
);

// Tool 6: gmail_count_unread
server.tool(
  "gmail_count_unread",
  "Count unread (unseen) emails across one or all mailboxes",
  {
    mailbox: z.string().optional().describe("Mailbox name (omit to count across all)"),
  },
  async ({ mailbox }) => {
    const mailboxes = mailbox ? [findMailbox(mailbox)] : config.mailboxes;
    const results: Array<{ mailbox: string; folder: string; unread: number }> = [];

    for (const mb of mailboxes) {
      try {
        const counts = await withImap(mb, async (client) => {
          const status = await client.status("INBOX", { unseen: true });
          return [{ mailbox: mb.name, folder: "INBOX", unread: status.unseen ?? 0 }];
        });
        results.push(...counts);
      } catch (err) {
        results.push({
          mailbox: mb.name,
          folder: "INBOX",
          unread: -1,
        });
      }
    }

    return { content: [{ type: "text" as const, text: JSON.stringify(results, null, 2) }] };
  }
);

// ---------------------------------------------------------------------------
// Start server
// ---------------------------------------------------------------------------

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  console.error("Gmail MCP server failed to start:", err);
  process.exit(1);
});
