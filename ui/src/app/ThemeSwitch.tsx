import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import { useColorScheme } from '@mui/material/styles';

export function ThemeSwitch() {
  const { mode, setMode } = useColorScheme();
  // mode is undefined on the very first render (before hydration settles).
  if (mode === undefined) return null;
  return (
    <ToggleButtonGroup
      exclusive
      size="small"
      aria-label="Colour theme"
      value={mode}
      onChange={(_event, next: 'light' | 'system' | 'dark' | null) => {
        if (next) setMode(next);
      }}
    >
      <ToggleButton value="light">Light</ToggleButton>
      <ToggleButton value="system">Auto</ToggleButton>
      <ToggleButton value="dark">Dark</ToggleButton>
    </ToggleButtonGroup>
  );
}
