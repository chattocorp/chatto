import { defineWebConfig } from "runling/web";
import chat from "./workflows/chat.ts";

export default defineWebConfig({
  webhooks: {
    chatto: chat.route,
  },
});
