/**
 * Load `.env` from the working directory, if it exists. Values already in the process environment
 * win. Import this module first: some dependencies read the environment when they load.
 */
try {
  process.loadEnvFile('.env');
} catch (error) {
  if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
}
