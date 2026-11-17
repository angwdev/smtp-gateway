import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import Button from '@mui/material/Button';
import { useRef, useState } from 'react';

export interface CopyButtonProps { value: string; label: string }

export function CopyButton({ value, label }: CopyButtonProps) {
  const [text, setText] = useState(label);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const onClick = () => {
    navigator.clipboard.writeText(value).then(
      () => {
        setText('Copied');
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setText(label), 2000);
      },
      () => setText('Select and copy manually'),
    );
  };

  return (
    <>
      <Button variant="outlined" size="small" startIcon={<ContentCopyIcon />} onClick={onClick}>
        {text}
      </Button>
      <span role="status" aria-live="polite" className="sr-only">{text === 'Copied' ? 'Copied' : ''}</span>
    </>
  );
}
