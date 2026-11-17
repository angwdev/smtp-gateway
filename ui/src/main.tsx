import { createRoot } from 'react-dom/client';

const root = document.getElementById('root');
if (!root) throw new Error('#root is missing from index.html');
createRoot(root).render(<p>SMTP Gateway</p>);
