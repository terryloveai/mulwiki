"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import { authKeys } from "@mulwiki/core/queries";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";

export default function RegisterPage() {
  return (
    <Suspense fallback={null}>
      <RegisterForm />
    </Suspense>
  );
}

function RegisterForm() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const searchParams = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [formError, setFormError] = useState("");
  const nextPath = safeNextPath(searchParams.get("next"));

  const mutation = useMutation({
    mutationFn: () => api.register({ email, password }),
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.me(), user);
      router.push(nextPath);
    },
  });

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    setFormError("");
    if (password !== confirm) {
      setFormError("Passwords do not match");
      return;
    }
    mutation.mutate();
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label htmlFor="email" className="text-sm font-medium text-foreground">
          Email
        </label>
        <Input
          id="email"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          required
        />
      </div>
      <div className="space-y-1.5">
        <label htmlFor="password" className="text-sm font-medium text-foreground">
          Password
        </label>
        <Input
          id="password"
          type="password"
          autoComplete="new-password"
          minLength={8}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />
      </div>
      <div className="space-y-1.5">
        <label htmlFor="confirm" className="text-sm font-medium text-foreground">
          Confirm password
        </label>
        <Input
          id="confirm"
          type="password"
          autoComplete="new-password"
          minLength={8}
          value={confirm}
          onChange={(event) => setConfirm(event.target.value)}
          required
        />
      </div>
      {(formError || mutation.isError) && (
        <p className="text-sm text-destructive">
          {formError || (mutation.error as Error).message}
        </p>
      )}
      <Button type="submit" variant="brand" className="w-full" disabled={mutation.isPending}>
        {mutation.isPending ? <Spinner className="h-4 w-4" /> : "Create account"}
      </Button>
      <p className="text-center text-sm text-muted-foreground">
        Already registered?{" "}
        <Link href={`/login?next=${encodeURIComponent(nextPath)}`} className="font-medium text-foreground hover:underline">
          Log in
        </Link>
      </p>
    </form>
  );
}

function safeNextPath(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//")) {
    return "/workspaces";
  }
  return value;
}
