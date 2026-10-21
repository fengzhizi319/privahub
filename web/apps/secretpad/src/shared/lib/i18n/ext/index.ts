/**
 * Extension dictionaries for the migration work. Each feature area owns one
 * file (`<area>.ts` exporting `{ zh, en }`) so parallel work does not collide
 * inside the big `dictionaries.ts`. Keys are deep-merged over the base dicts.
 */
import type { Dictionary } from '../dictionaries';
import * as platform from './platform';
import * as projects from './projects';
import * as data from './data';
import * as workflow from './workflow';
import * as nodes from './nodes';
import * as dag from './dag';
import * as audit from './audit';
import * as authExt from './auth';

const areas: { zh: Dictionary; en: Dictionary }[] = [platform, projects, data, workflow, nodes, dag, audit, authExt];

export const extZh: Dictionary[] = areas.map((a) => a.zh);
export const extEn: Dictionary[] = areas.map((a) => a.en);
