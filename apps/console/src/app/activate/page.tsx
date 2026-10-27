import { AuthFrame } from "@/components/auth/frame";
import { ActivateForm } from "@/components/auth/forms";
export const metadata = { title: "Activate account" };
export default function Activate() {
  return (
    <AuthFrame>
      <h1>Set your password</h1>
      <ActivateForm />
    </AuthFrame>
  );
}
