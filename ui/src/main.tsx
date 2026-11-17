import { createRoot } from 'react-dom/client';
import { StrictMode } from 'react';

import { App } from './app/App';

const root = document.getElementById('root');
if (!root) throw new Error('#root is missing from index.html');
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
