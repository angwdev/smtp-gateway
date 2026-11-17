import { lazy } from 'react';
import { Navigate, Route, Routes, useParams } from 'react-router';

import { NotFound } from '../pages/NotFound';
import { SignedOut } from '../pages/SignedOut';
import { AuthGate } from './AuthGate';
import { Shell } from './Shell';

const Overview = lazy(() => import('../pages/Overview').then((m) => ({ default: m.Overview })));
const Logins = lazy(() => import('../pages/Logins').then((m) => ({ default: m.Logins })));
const LoginNew = lazy(() => import('../pages/LoginNew').then((m) => ({ default: m.LoginNew })));
const LoginDetail = lazy(() => import('../pages/LoginDetail').then((m) => ({ default: m.LoginDetail })));
const Messages = lazy(() => import('../pages/Messages').then((m) => ({ default: m.Messages })));
const MessageDetail = lazy(() => import('../pages/MessageDetail').then((m) => ({ default: m.MessageDetail })));
const Audit = lazy(() => import('../pages/Audit').then((m) => ({ default: m.Audit })));
const Tokens = lazy(() => import('../pages/Tokens').then((m) => ({ default: m.Tokens })));
const More = lazy(() => import('../pages/More').then((m) => ({ default: m.More })));

function CredentialRedirect({ suffix }: { suffix: string }) {
  const { id } = useParams();
  return <Navigate replace to={`/logins/${suffix}${id ?? ''}`} />;
}

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/signed-out" element={<SignedOut />} />
      <Route element={<AuthGate><Shell /></AuthGate>}>
        <Route index element={<Overview />} />
        <Route path="logins" element={<Logins />} />
        <Route path="logins/new" element={<LoginNew />} />
        <Route path="logins/:id" element={<LoginDetail />} />
        <Route path="messages" element={<Messages />} />
        <Route path="messages/:id" element={<MessageDetail />} />
        <Route path="audit" element={<Audit />} />
        <Route path="tokens" element={<Tokens />} />
        <Route path="more" element={<More />} />
        <Route path="logs" element={<Navigate replace to="/messages" />} />
        <Route path="credentials/new" element={<Navigate replace to="/logins/new" />} />
        <Route path="credentials/:id" element={<CredentialRedirect suffix="" />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}
