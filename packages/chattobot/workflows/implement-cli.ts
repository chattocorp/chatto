/** Entry point for running the implementation task with `runling run`, outside a Chatto
 * conversation. Settings come from the same environment variables as the bot. Runs started here
 * have no owner key, so the bot cannot resume their artifacts, and they cannot resume the bot's. */
import { ConfigurationError } from '../settings.ts';
import { createImplementation, implementationSettings } from './implement.ts';

const settings = implementationSettings();
if (!settings)
  throw new ConfigurationError(
    'Set CHATTO_IMPLEMENTATION_REPOSITORY and CHATTO_SOURCE_DIRECTORY to run implementations'
  );

export default createImplementation(settings);
