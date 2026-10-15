"use client";

import type { ReactNode } from "react";
import { Select as SelectPrimitive } from "@base-ui/react/select";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SelectOption {
  value: string;
  label: ReactNode;
  disabled?: boolean;
}

interface SelectProps {
  value: string;
  options: SelectOption[];
  onValueChange: (value: string) => void;
  id?: string;
  name?: string;
  disabled?: boolean;
  required?: boolean;
  placeholder?: string;
  className?: string;
  "aria-label"?: string;
  "aria-describedby"?: string;
}

export function Select({
  value,
  options,
  onValueChange,
  id,
  name,
  disabled,
  required,
  placeholder = "请选择",
  className,
  ...ariaProps
}: SelectProps) {
  const selected = options.find((option) => option.value === value);
  return (
    <SelectPrimitive.Root
      value={selected?.value ?? null}
      items={options}
      name={name}
      disabled={disabled}
      required={required}
      onValueChange={(next) => {
        if (next !== null) onValueChange(next);
      }}
    >
      <SelectPrimitive.Trigger
        id={id}
        {...ariaProps}
        className={cn(
          "flex min-h-9 w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-left text-sm text-zinc-800 outline-none transition-colors hover:border-zinc-300 focus-visible:border-blue-400 focus-visible:ring-2 focus-visible:ring-blue-100 disabled:cursor-not-allowed disabled:bg-zinc-50 disabled:text-zinc-400",
          className,
        )}
      >
        <SelectPrimitive.Value className="min-w-0 flex-1 truncate">
          {() => selected?.label ?? placeholder}
        </SelectPrimitive.Value>
        <SelectPrimitive.Icon>
          <ChevronDown className="size-4 shrink-0 text-zinc-400" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Positioner
          className="z-[100] outline-none"
          sideOffset={6}
          align="start"
          alignItemWithTrigger={false}
          collisionPadding={12}
        >
          <SelectPrimitive.Popup className="max-h-[min(20rem,var(--available-height))] w-[var(--anchor-width)] min-w-40 max-w-[calc(100vw-1.5rem)] overflow-y-auto overscroll-contain rounded-lg border border-zinc-200 bg-white p-1 text-sm shadow-lg outline-none">
            <SelectPrimitive.List>
              {options.map((option) => (
                <SelectPrimitive.Item
                  key={option.value}
                  value={option.value}
                  disabled={option.disabled}
                  className="flex cursor-default items-center gap-3 rounded-md px-3 py-2 text-zinc-700 outline-none data-[highlighted]:bg-blue-50 data-[highlighted]:text-blue-700 data-[selected]:bg-blue-50 data-[selected]:font-medium data-[selected]:text-blue-700 data-[disabled]:pointer-events-none data-[disabled]:opacity-40"
                >
                  <SelectPrimitive.ItemText className="min-w-0 flex-1 break-words">
                    {option.label}
                  </SelectPrimitive.ItemText>
                  <span className="flex size-4 shrink-0 items-center justify-center">
                    <SelectPrimitive.ItemIndicator>
                      <Check className="size-4" />
                    </SelectPrimitive.ItemIndicator>
                  </span>
                </SelectPrimitive.Item>
              ))}
            </SelectPrimitive.List>
          </SelectPrimitive.Popup>
        </SelectPrimitive.Positioner>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}
