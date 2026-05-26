---
name: "source-command-chatto-overview"
description: "Give a high-level overview of Chatto's GraphQL schema, NATS messaging, and frontend structure."
---

# source-command-chatto-overview

Use this skill when the user asks for a high-level overview of the Chatto codebase, especially its GraphQL schema, NATS messaging layout, or frontend structure.

## Command Template

Please examine the codebase and give me a high-level overview of its structure and organization.

Please sort it into the following sections, answering the noted questions:

## GraphQL schema

- What top-level queries, mutations and subscriptions are defined?
- What do their resolvers do?
- What are the most important types and relationships in the schema?

# NATS messaging

- What streams, KV buckets and object stores do we have defined?
- What data do they hold, and how is that data structured?
- What is the structure of the application's subject space?

# Frontend

- How is the frontend application structured?
- What are the most important components and their responsibilities?
- How do they interact with the backend?

## Migration Note

This was migrated from the Claude source command `chatto-overview`. The source command did not use arguments, file expansion, or shell interpolation, so no provider-specific runtime behavior needs to be preserved.
