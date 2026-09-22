import { defineWebConfig } from "runling/web";
import { chattoSource } from "./chatto/realtime.ts";

export default defineWebConfig({
  sources: { chatto: chattoSource },
});
