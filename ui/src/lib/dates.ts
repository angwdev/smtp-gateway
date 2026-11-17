import { AdapterDayjs } from '@mui/x-date-pickers/AdapterDayjs';
import { DatePicker } from '@mui/x-date-pickers/DatePicker';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import { createElement } from 'react';
// This is the one other file (besides format.ts) allowed to import Day.js
// directly: the picker itself needs the real library, not the helpers.
import dayjs, { type Dayjs } from 'dayjs';

export interface DateFieldProps {
  label: string;
  value: string | null;
  onChange: (day: string | null) => void;
}

// Kept as a .ts file (no JSX) per the plan; createElement avoids needing .tsx.
export function DateField({ label, value, onChange }: DateFieldProps) {
  return createElement(
    LocalizationProvider,
    { dateAdapter: AdapterDayjs },
    createElement(DatePicker, {
      label,
      value: value ? dayjs(value) : null,
      onChange: (day: Dayjs | null) => onChange(day ? day.format('YYYY-MM-DD') : null),
      maxDate: dayjs(),
      slotProps: { textField: { size: 'small' }, field: { clearable: true } },
    }),
  );
}
