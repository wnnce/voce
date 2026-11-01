import React from 'react';
import type { WidgetProps } from '@rjsf/utils';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';

export const CompactInputWidget: React.FC<WidgetProps> = ({
  id,
  value,
  readonly,
  disabled,
  type,
  onChange,
  onBlur,
  onFocus,
  placeholder,
  schema,
  rawErrors,
}) => {
  const isNumber = schema.type === 'number' || schema.type === 'integer';

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    if (isNumber) {
      if (val === '') {
        onChange(undefined);
      } else {
        const num = schema.type === 'integer' ? parseInt(val, 10) : parseFloat(val);
        onChange(Number.isNaN(num) ? val : num);
      }
    } else {
      onChange(val === '' ? options.emptyValue : val);
    }
  };

  const options = schema as { emptyValue?: unknown };

  return (
    <Input
      id={id}
      type={type || (isNumber ? 'number' : 'text')}
      value={value ?? ''}
      placeholder={placeholder}
      disabled={disabled || readonly}
      onChange={handleChange}
      onBlur={onBlur && (() => onBlur(id, value))}
      onFocus={onFocus && (() => onFocus(id, value))}
      className={cn(
        'h-7 text-xs bg-sunken/40 border-border/80 focus:bg-card',
        isNumber && 'font-mono',
        rawErrors && rawErrors.length > 0 && 'border-destructive focus-visible:ring-destructive'
      )}
    />
  );
};

export const CompactTextareaWidget: React.FC<WidgetProps> = ({
  id,
  value,
  readonly,
  disabled,
  onChange,
  onBlur,
  onFocus,
  placeholder,
  rawErrors,
}) => {
  return (
    <Textarea
      id={id}
      value={value ?? ''}
      placeholder={placeholder}
      disabled={disabled || readonly}
      rows={3}
      onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
        onChange(e.target.value === '' ? undefined : e.target.value)
      }
      onBlur={onBlur && (() => onBlur(id, value))}
      onFocus={onFocus && (() => onFocus(id, value))}
      className={cn(
        'text-xs bg-sunken/40 border-border/80 focus:bg-card min-h-[50px]',
        rawErrors && rawErrors.length > 0 && 'border-destructive'
      )}
    />
  );
};

export const CompactCheckboxWidget: React.FC<WidgetProps> = ({
  id,
  value,
  disabled,
  readonly,
  onChange,
  label,
}) => {
  return (
    <div className="flex items-center gap-2 py-0.5">
      <Switch
        id={id}
        checked={Boolean(value)}
        disabled={disabled || readonly}
        onCheckedChange={(checked) => onChange(checked)}
      />
      {label && (
        <label htmlFor={id} className="text-xs text-foreground cursor-pointer select-none">
          {label}
        </label>
      )}
    </div>
  );
};

export const CompactSelectWidget: React.FC<WidgetProps> = ({
  id,
  value,
  disabled,
  readonly,
  onChange,
  options,
  placeholder,
  rawErrors,
}) => {
  const { enumOptions } = options as {
    enumOptions?: Array<{ value: unknown; label: string }>;
  };

  const selectedValue = value !== undefined && value !== null ? String(value) : '';

  return (
    <Select
      value={selectedValue}
      disabled={disabled || readonly}
      onValueChange={(val) => {
        const option = enumOptions?.find((o) => String(o.value) === val);
        onChange(option ? option.value : val);
      }}
    >
      <SelectTrigger
        id={id}
        className={cn(
          'h-7 text-xs bg-sunken/40 border-border/80 focus:bg-card',
          rawErrors && rawErrors.length > 0 && 'border-destructive'
        )}
      >
        <SelectValue placeholder={placeholder || 'Select...'} />
      </SelectTrigger>
      <SelectContent>
        {enumOptions?.map((opt, i) => (
          <SelectItem key={i} value={String(opt.value)}>
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
};
