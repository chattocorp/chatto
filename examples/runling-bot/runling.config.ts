import { defineWebConfig } from "runling/web";
import reply from "./reply.ts";

export default defineWebConfig({
  webhooks: { chatto: { workflow: reply } },
});
