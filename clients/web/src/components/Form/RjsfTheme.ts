import type { RegistryWidgetsType, TemplatesType } from '@rjsf/utils';
import {
  CompactInputWidget,
  CompactTextareaWidget,
  CompactCheckboxWidget,
  CompactSelectWidget,
} from './CompactWidgets';
import {
  CompactFieldTemplate,
  CompactObjectFieldTemplate,
  CompactArrayFieldTemplate,
} from './CompactTemplates';

export const compactWidgets: RegistryWidgetsType = {
  TextWidget: CompactInputWidget,
  EmailWidget: CompactInputWidget,
  URLWidget: CompactInputWidget,
  PasswordWidget: CompactInputWidget,
  ColorWidget: CompactInputWidget,
  UpDownWidget: CompactInputWidget,
  NumberWidget: CompactInputWidget,
  IntegerWidget: CompactInputWidget,
  TextareaWidget: CompactTextareaWidget,
  CheckboxWidget: CompactCheckboxWidget,
  SelectWidget: CompactSelectWidget,
};

export const compactTemplates: Partial<TemplatesType> = {
  FieldTemplate: CompactFieldTemplate,
  ObjectFieldTemplate: CompactObjectFieldTemplate,
  ArrayFieldTemplate: CompactArrayFieldTemplate,
  BaseInputTemplate: CompactInputWidget,
};
