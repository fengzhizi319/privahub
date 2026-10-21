import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/mocks/server';
import { withDataResourceOwner, ownerFieldFor } from './data-resource';
import { listDatasourcesJava } from './ext/data';
import { createApprovalJava } from './ext/p2p';

const autonomy = { ownerId: 'inst-a', ownerType: 'P2P' };
const center = { ownerId: 'kuscia-system', ownerType: 'CENTER' };

describe('withDataResourceOwner (@DataResource owner-scoped routes)', () => {
  it('fills the missing owner field for non-CENTER accounts', () => {
    expect(withDataResourceOwner('/api/v1alpha1/datasource/list', { page: 1 }, autonomy)).toEqual({ page: 1, ownerId: 'inst-a' });
    expect(withDataResourceOwner('message/pending', undefined, autonomy)).toEqual({ ownerId: 'inst-a' });
    expect(withDataResourceOwner('approval/create', { voteType: 'X' }, autonomy)).toEqual({ voteType: 'X', initiatorId: 'inst-a' });
    expect(withDataResourceOwner('message/reply', { voteId: 'v' }, autonomy)).toEqual({ voteId: 'v', voteParticipantId: 'inst-a' });
    expect(withDataResourceOwner('inst/get', {}, autonomy)).toEqual({ instId: 'inst-a' });
  });

  it('keeps an explicit id (camelCase or snake_case) and treats blank as missing', () => {
    const body = { ownerId: 'node-b' };
    expect(withDataResourceOwner('datasource/list', body, autonomy)).toBe(body);
    const snake = { owner_id: 'node-b' };
    expect(withDataResourceOwner('datasource/list', snake, autonomy)).toBe(snake);
    expect(withDataResourceOwner('datasource/list', { ownerId: ' ' }, autonomy)).toEqual({ ownerId: 'inst-a' });
  });

  it('never touches CENTER owners, unlisted routes or target-id routes', () => {
    expect(ownerFieldFor('datasource/list', center)).toBeNull();
    expect(ownerFieldFor('project/get', autonomy)).toBeNull();
    expect(ownerFieldFor('node/get', autonomy)).toBeNull();
    expect(ownerFieldFor('datasource/list', null)).toBeNull();
  });
});

describe('api middleware sends the owner id on the wire', () => {
  it('adds ownerId / initiatorId to real requests of a logged-in AUTONOMY user', async () => {
    localStorage.setItem('secretpad-user', JSON.stringify(autonomy));
    const bodies: Record<string, unknown> = {};
    server.use(
      http.post('*/api/v1alpha1/datasource/list', async ({ request }) => {
        bodies.list = await request.json();
        return HttpResponse.json({ status: { code: 0 }, data: { infos: [], total: 0 } });
      }),
      http.post('*/api/v1alpha1/approval/create', async ({ request }) => {
        bodies.approval = await request.json();
        return HttpResponse.json({ status: { code: 0 }, data: null });
      }),
    );
    await listDatasourcesJava({} as never);
    await createApprovalJava({ voteType: 'PROJECT_CREATE' } as never);
    expect(bodies.list).toMatchObject({ ownerId: 'inst-a' });
    expect(bodies.approval).toMatchObject({ initiatorId: 'inst-a', voteType: 'PROJECT_CREATE' });
  });

  it('leaves CENTER requests unchanged', async () => {
    localStorage.setItem('secretpad-user', JSON.stringify(center));
    let body: unknown;
    server.use(
      http.post('*/api/v1alpha1/datasource/list', async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ status: { code: 0 }, data: { infos: [], total: 0 } });
      }),
    );
    await listDatasourcesJava({ page: 1 } as never);
    expect(body).toEqual({ page: 1 });
  });
});
