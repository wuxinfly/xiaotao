#!/usr/bin/env node

import { run } from '../cli/index.js';

const exitCode = await run(process.argv.slice(2));
process.exitCode = exitCode;
