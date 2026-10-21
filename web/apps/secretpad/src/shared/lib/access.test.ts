import { describe, it, expect } from 'vitest';
import { canAccessPath, getMenu, homeHref, resolveHomePath } from './access';
import { toPlatformContext, type PlatformUser } from './platform';

const ctx = (u: PlatformUser) => toPlatformContext(u);
const center = ctx({ platformType: 'CENTER', ownerType: 'CENTER', ownerId: 'kuscia-system' });
const edgeOnCenter = ctx({ platformType: 'CENTER', ownerType: 'EDGE', ownerId: 'alice' });
const edge = ctx({ platformType: 'EDGE', ownerType: 'EDGE', ownerId: 'alice' });
const autonomy = ctx({ platformType: 'AUTONOMY', ownerType: 'P2P', ownerId: 'inst-a' });
const p2p = ctx({ platformType: 'P2P', ownerId: 'bob' });
const test = ctx({ platformType: 'TEST' });

describe('resolveHomePath', () => {
  it('sends AUTONOMY to the workbench with its ownerId', () => {
    expect(homeHref(resolveHomePath(autonomy))).toBe('/workbench?ownerId=inst-a');
  });
  it('sends EDGE to its node view', () => {
    expect(homeHref(resolveHomePath(edge))).toBe('/node/alice');
  });
  it('sends CENTER admins to /guide on first login, dashboard afterwards', () => {
    expect(resolveHomePath(center, { firstLogin: true }).to).toBe('/guide');
    expect(resolveHomePath(center).to).toBe('/dashboard');
  });
  it('never sends EDGE accounts on CENTER to the guide', () => {
    expect(resolveHomePath(edgeOnCenter, { firstLogin: true }).to).toBe('/dashboard');
  });
  it('sends P2P to the workbench and TEST to the dashboard', () => {
    expect(resolveHomePath(p2p).to).toBe('/workbench');
    expect(resolveHomePath(test).to).toBe('/dashboard');
  });
});

describe('canAccessPath', () => {
  it('hides node management and guide from EDGE accounts on CENTER', () => {
    expect(canAccessPath('/nodes', edgeOnCenter)).toBe(false);
    expect(canAccessPath('/guide', edgeOnCenter)).toBe(false);
    expect(canAccessPath('/all-data-tables', edgeOnCenter)).toBe(false);
    expect(canAccessPath('/projects', edgeOnCenter)).toBe(true);
    expect(canAccessPath('/nodes', center)).toBe(true);
  });

  it('restricts the node view to own node except for CENTER admins', () => {
    expect(canAccessPath('/node/alice/data-tables', edge)).toBe(true);
    expect(canAccessPath('/node/bob/data-tables', edge)).toBe(false);
    expect(canAccessPath('/node/bob', center)).toBe(true);
    expect(canAccessPath('/node/alice', edgeOnCenter)).toBe(true);
    expect(canAccessPath('/node/bob', edgeOnCenter)).toBe(false);
    expect(canAccessPath('/node/inst-a/results', autonomy)).toBe(true);
    expect(canAccessPath('/node/other', autonomy)).toBe(false);
  });

  it('keeps EDGE out of center collaboration pages', () => {
    expect(canAccessPath('/projects', edge)).toBe(false);
    expect(canAccessPath('/dag', edge)).toBe(false);
    expect(canAccessPath('/dashboard', edge)).toBe(false);
    expect(canAccessPath('/messages', edge)).toBe(true);
    expect(canAccessPath('/p2p/my-node', edge)).toBe(true);
  });

  it('keeps CENTER off the institution pages (no inst on CENTER → INST_NOT_EXISTS)', () => {
    expect(canAccessPath('/institutions', center)).toBe(false);
    expect(canAccessPath('/inst-register', center)).toBe(false);
    expect(canAccessPath('/institutions', autonomy)).toBe(true);
    expect(canAccessPath('/institutions', p2p)).toBe(true);
  });

  it('lets AUTONOMY/P2P use DAG, records and p2p projects but not center projects', () => {
    for (const c of [autonomy, p2p]) {
      expect(canAccessPath('/dag', c)).toBe(true);
      expect(canAccessPath('/job-records', c)).toBe(true);
      expect(canAccessPath('/p2p/projects', c)).toBe(true);
      expect(canAccessPath('/projects', c)).toBe(false);
      expect(canAccessPath('/nodes', c)).toBe(false);
    }
    expect(canAccessPath('/p2p/projects', center)).toBe(false);
  });

  it('allows unknown paths and everything on TEST', () => {
    expect(canAccessPath('/something-new', edge)).toBe(true);
    expect(canAccessPath('/nodes', test)).toBe(true);
  });
});

describe('getMenu', () => {
  const paths = (c: ReturnType<typeof ctx>) => getMenu(c).filter((m) => m.path).map((m) => m.path);

  it.each([
    ['center', center],
    ['edgeOnCenter', edgeOnCenter],
    ['edge', edge],
    ['autonomy', autonomy],
    ['p2p', p2p],
    ['test', test],
  ])('only lists routes the %s account can open', (_name, c) => {
    for (const item of getMenu(c).filter((m) => m.path)) {
      expect(canAccessPath(homeHref({ to: item.path!, params: item.params }), c)).toBe(true);
    }
  });

  it('hides node management for EDGE accounts on CENTER and shows own node view', () => {
    expect(paths(edgeOnCenter)).not.toContain('/nodes');
    expect(paths(edgeOnCenter)).toContain('/node/$nodeId');
    expect(paths(center)).toContain('/nodes');
    expect(paths(center)).toContain('/all-data-sources');
  });

  it('gives P2P/AUTONOMY a results entry and the EDGE node-view tabs', () => {
    expect(paths(autonomy)).toContain('/results');
    expect(paths(p2p)).toContain('/p2p/projects');
    expect(paths(edge)).toEqual(
      expect.arrayContaining(['/node/$nodeId/data-sources', '/node/$nodeId/results', '/messages']),
    );
  });
});


it('provides separate custom/predefined navigation with matching access rules', () => {
  for (const context of [center, autonomy, p2p, test]) {
    const menu = getMenu(context);
    expect(menu.filter(item => item.path === '/dag')).toHaveLength(1);
    expect(menu.filter(item => item.path === '/dag-templates')).toHaveLength(1);
    expect(canAccessPath('/dag-templates', context)).toBe(canAccessPath('/dag', context));
  }
  expect(canAccessPath('/dag-templates', edge)).toBe(false);
});

it.each([center, edgeOnCenter, edge, autonomy, p2p, test])('groups authorized navigation without duplicates or empty sections (%s)', context => {
 const menu=getMenu(context);
 const paths=menu.filter(item=>item.path).map(item=>item.path);
 expect(new Set(paths).size).toBe(paths.length);
 for(let index=0;index<menu.length;index++){
  if(menu[index].section) expect(menu[index+1]?.path).toBeTruthy();
 }
 if(paths.includes('/dag')){
  const start=paths.indexOf('/dag');
  expect(paths[start+1]).toBe('/dag-templates');
  expect(paths.indexOf('/job-records')).toBeGreaterThan(start);
 }
});

it('places task execution before outputs and separates node management from data resources',()=>{
 const menu=getMenu(center);
 const grouped:Record<string,string[]>={};let section='';
 for(const item of menu){if(item.section){section=item.section;grouped[section]=[];}else grouped[section].push(item.path!);}
 expect(grouped['sidebar.tasksSection']).toEqual(['/job-records','/periodic-tasks','/results','/models']);
 expect(grouped['sidebar.startSection']).toContain('/privacy-scenes');
 expect(grouped['sidebar.networkSection']).toContain('/nodes');
 expect(grouped['sidebar.dataSection']).not.toContain('/nodes');
 expect(grouped['sidebar.platformSection'].at(-1)).toBe('/account');
});

it('highlights workbench links with owner parameters and avoids similar route prefixes', async()=>{
 const {isMenuActive}=await import('./access');
 expect(isMenuActive('/workbench','/workbench?ownerId=alice')).toBe(true);
 expect(isMenuActive('/dag-templates','/dag')).toBe(false);
 expect(isMenuActive('/projects/detail','/projects')).toBe(true);
});
