import React from 'react';
import type {
  FieldTemplateProps,
  ObjectFieldTemplateProps,
  ArrayFieldTemplateProps,
  ObjectFieldTemplatePropertyType,
} from '@rjsf/utils';
import { HelpCircle, Plus } from 'lucide-react';
import { Tooltip } from '@/components/ui/tooltip';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export const CompactFieldTemplate: React.FC<FieldTemplateProps> = ({
  id,
  classNames,
  label,
  rawDescription,
  children,
  rawErrors,
  help,
  hidden,
  required,
  displayLabel,
  schema,
}) => {
  if (hidden) return <div className="hidden">{children}</div>;

  const isCheckbox = schema.type === 'boolean';

  return (
    <div className={cn('flex flex-col gap-1 mb-2', classNames)}>
      {displayLabel && label && !isCheckbox && (
        <div className="flex items-center justify-between">
          <label htmlFor={id} className="text-[11px] font-medium text-muted-foreground flex items-center gap-1 select-none">
            <span>{label}</span>
            {required && <span className="text-destructive">*</span>}
          </label>
          {rawDescription && (
            <Tooltip title={rawDescription}>
              <span className="text-muted-foreground/60 hover:text-foreground cursor-help p-0.5">
                <HelpCircle className="h-3 w-3" />
              </span>
            </Tooltip>
          )}
        </div>
      )}

      <div>{children}</div>

      {rawErrors && rawErrors.length > 0 && (
        <div className="text-[10px] text-destructive font-medium leading-none mt-0.5">
          {rawErrors[0]}
        </div>
      )}

      {help && <div className="text-[10px] text-muted-foreground">{help}</div>}
    </div>
  );
};

export const CompactObjectFieldTemplate: React.FC<ObjectFieldTemplateProps> = ({
  title,
  description,
  properties,
  fieldPathId,
}) => {
  const isRoot = !fieldPathId || !fieldPathId.path || fieldPathId.path.length === 0;

  return (
    <div className={cn(!isRoot && 'border border-border/60 rounded-lg p-2.5 bg-sunken/20 mb-2')}>
      {!isRoot && title && (
        <div className="text-xs font-semibold text-foreground mb-1.5 flex items-center justify-between border-b border-border/40 pb-1">
          <span>{title}</span>
          {description && typeof description === 'string' && (
            <Tooltip title={description}>
              <HelpCircle className="h-3 w-3 text-muted-foreground" />
            </Tooltip>
          )}
        </div>
      )}
      <div className={cn(isRoot ? 'grid grid-cols-1 md:grid-cols-2 gap-x-3 gap-y-1' : 'flex flex-col gap-1')}>
        {properties.map((prop: ObjectFieldTemplatePropertyType) => {
          const propSchema = (prop.content?.props as { schema?: { type?: string } } | undefined)?.schema;
          const isNestedObject = propSchema?.type === 'object';
          return (
            <div key={prop.name} className={cn(isNestedObject && 'md:col-span-2')}>
              {prop.content}
            </div>
          );
        })}
      </div>
    </div>
  );
};

export const CompactArrayFieldTemplate: React.FC<ArrayFieldTemplateProps> = ({
  title,
  items,
  canAdd,
  onAddClick,
  disabled,
  readonly,
}) => {
  return (
    <div className="border border-border/60 rounded-lg p-2.5 bg-sunken/20 mb-2">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-semibold text-foreground">{title}</span>
        {canAdd && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onAddClick}
            disabled={disabled || readonly}
            className="h-6 text-[11px] px-2 gap-1"
          >
            <Plus className="h-3 w-3" /> Add
          </Button>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        {items.map((item, idx) => (
          <div key={idx} className="flex items-start gap-1.5">
            <div className="flex-1 min-w-0">{item}</div>
          </div>
        ))}
      </div>
    </div>
  );
};
