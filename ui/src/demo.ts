import type { AgentEvent } from './lib';

export function demoEvents(): AgentEvent[] {
  const start = Date.now() - 24000;
  const rows: Partial<AgentEvent>[] = [
    { node_name: 'cmd/server/main.go', action: 'scouted', summary: 'Mapped the request entry point', why: 'I traced incoming requests before changing the server boundary.' },
    { node_name: 'internal/api/router.go#NewRouter', action: 'modified', summary: 'Connected the API route layer', why: 'Keeping route registration in one place makes the request path easier to follow.', parents: ['cmd/server/main.go'] },
    { node_name: 'internal/auth/session.go#Session', action: 'created', summary: 'Defined a typed session boundary', why: 'A small session type keeps authentication details out of the handlers.', parents: ['internal/api/router.go#NewRouter'], kind: 'type', after: 'type Session struct {\n    UserID string\n    ExpiresAt time.Time\n}', diff: '@@ -0,0 +1,4 @@\n+type Session struct {\n+    UserID string\n+    ExpiresAt time.Time\n+}' },
    { node_name: 'internal/auth/verify.go#Verify', action: 'created', summary: 'Verified tokens before handler execution', why: 'Rejecting expired tokens at the boundary keeps protected handlers simple.', parents: ['internal/auth/session.go#Session'], kind: 'function', after: 'func Verify(s Session) error {\n    if time.Now().After(s.ExpiresAt) {\n        return ErrExpired\n    }\n    return nil\n}', diff: '@@ -0,0 +1,6 @@\n+func Verify(s Session) error {\n+    if time.Now().After(s.ExpiresAt) {\n+        return ErrExpired\n+    }\n+    return nil\n+}' },
    { agent_id: 'test-agent', node_name: 'internal/auth/verify_test.go#TestExpired', action: 'created', summary: 'Covered the expired token case', why: 'The expiry boundary needs an explicit regression check.', parents: ['internal/auth/verify.go#Verify'] },
    { node_name: 'internal/api/middleware.go#Authenticate', action: 'modified', summary: 'Added authentication to protected routes', why: 'Middleware applies the same session check to every protected route.', parents: ['internal/auth/verify.go#Verify'] },
    { agent_id: 'test-agent', node_name: 'internal/api/middleware.go#Authenticate', action: 'modified', summary: 'Adjusted the test injection point', why: 'I need a deterministic verifier to exercise the middleware error path.', collisions: ['claude-main'] },
    { agent_id: 'test-agent', node_name: 'internal/api/middleware.go#Authenticate', action: 'tested', summary: 'Released the shared middleware file', why: 'I finished the test adjustment so the implementation agent can continue.', status: 'ready' },
    { node_name: 'internal/api/handlers.go#GetProfile', action: 'modified', summary: 'Read identity from validated sessions', why: 'Handlers can use the validated identity without parsing tokens again.', parents: ['internal/api/middleware.go#Authenticate'] },
    { agent_id: 'test-agent', node_name: 'internal/auth/verify_test.go#TestExpired', action: 'tested', summary: 'Authentication regression checks all passed', why: 'The tests confirm expired tokens are rejected and valid sessions reach the handler.', status: 'ready' },
    { node_name: 'cmd/server/main.go', action: 'modified', summary: 'Wired authentication into the server', why: 'The entry point owns dependency assembly, so the verifier is injected here.', parents: ['internal/api/handlers.go#GetProfile'], status: 'ready' },
  ];
  return rows.map((row, index) => ({ id: String(index + 1), timestamp: new Date(start + index * 2000).toISOString(), agent_id: 'claude-main', node_name: '', action: 'modified', summary: '', why: '', status: 'running', source: 'demo', language: 'go', kind: 'function', ...row }));
}
