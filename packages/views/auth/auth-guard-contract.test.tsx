import type { ComponentProps } from "react";
import type { ComponentType } from "react";
import { AccountView } from "./AccountView";
import { AuthGuard } from "./AuthGuard";

const _props: ComponentProps<typeof AuthGuard> = {
  children: null,
};

function expectComponent(_component: ComponentType) {}

expectComponent(AccountView);
