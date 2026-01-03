import {
  createEffect,
  createResource,
  createSignal,
  For,
  Match,
  onMount,
  Show,
  Switch,
} from "solid-js";
import "./App.css";
import { Settings } from "../bindings/resocks5/internal/state";
import {
  GetSettings,
  SaveSettings,
  StartProxy,
  StopProxy,
  IsProxyRunning,
  GetLocalAddress,
} from "../bindings/resocks5/appservice";
import { sleep } from "./utils/sleep";
import { FaRegularCopy } from "solid-icons/fa";
import { Events } from "@wailsio/runtime";

const connectedText = "Делаем";
const notConnectedText = "Запустить";
const startingText = "Стартуем...";

export const App = () => {
  const [isSettingsOpen, setIsSettingsOpen] = createSignal(false);

  return (
    <Switch>
      <Match when={isSettingsOpen()}>
        <SettingsScreen OnClickBack={() => setIsSettingsOpen(false)} />
      </Match>
      <Match when={!isSettingsOpen()}>
        <MainScreen OnClickSettings={() => setIsSettingsOpen(true)} />
      </Match>
    </Switch>
  );
};

const SettingsScreen = (props: { OnClickBack: () => void }) => {
  const [settings] = createResource(GetSettings);
  const [isSaving, setIsSaving] = createSignal(false);

  const handleSave = async (event: SubmitEvent) => {
    event.preventDefault();

    const formData = new FormData(event.target as HTMLFormElement);

    const formValues: Settings = {
      enabled: settings()?.enabled || false,
      serverAddress: formData.get("serverAddress") as string,
      serverPort: +(formData.get("serverPort") as string),
      serverLogin: formData.get("serverLogin") as string,
      serverPassword: formData.get("serverPassword") as string,
    };

    setIsSaving(true);

    try {
      await SaveSettings(formValues);
      await sleep(1000);
      props.OnClickBack();
    } catch (error) {
      console.error(error);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <form class="p-3" onSubmit={handleSave}>
      <div class="flex justify-between gap-2">
        <button
          type="button"
          class="btn btn-xs btn-neutral"
          onClick={props.OnClickBack}
          disabled={isSaving()}
        >
          Назад
        </button>

        <button
          class="btn btn-xs btn-primary"
          type="submit"
          disabled={isSaving()}
        >
          {isSaving() ? "Сохраняем..." : "Сохранить"}
        </button>
      </div>

      <div class="mt-6">
        <fieldset class="fieldset">
          <legend class="fieldset-legend text-xs">Адрес SOCKS5 сервера</legend>
          <input
            required
            name="serverAddress"
            type="text"
            class="input"
            placeholder="141.141.141.141"
            value={settings()?.serverAddress || ""}
          />
        </fieldset>

        <fieldset class="fieldset">
          <legend class="fieldset-legend text-xs">Порт SOCKS5 сервера</legend>
          <input
            required
            name="serverPort"
            type="number"
            class="input input-number-no-spinner"
            placeholder="1080"
            value={settings()?.serverPort || ""}
          />
        </fieldset>

        <fieldset class="fieldset">
          <legend class="fieldset-legend text-xs">Логин</legend>
          <input
            required
            name="serverLogin"
            type="text"
            class="input"
            placeholder="admin"
            value={settings()?.serverLogin || ""}
          />
        </fieldset>

        <fieldset class="fieldset">
          <legend class="fieldset-legend text-xs">Пароль</legend>
          <input
            required
            name="serverPassword"
            type="password"
            class="input"
            placeholder="123456"
            value={settings()?.serverPassword || ""}
          />
        </fieldset>
      </div>
    </form>
  );
};

const MainScreen = (props: { OnClickSettings: () => void }) => {
  const [isConnected, setIsConnected] = createSignal(false);
  const [serverAddress] = createResource(GetLocalAddress);
  const [isStarting, setIsStarting] = createSignal(false);

  createEffect(() => {
    IsProxyRunning().then(setIsConnected).catch(console.error);
  });

  onMount(() => {
    Events.On("proxy-started", () => {
      setIsConnected(true);
    });
    Events.On("proxy-stopped", () => {
      setIsConnected(false);
    });
  });

  const handleToggleProxy = async () => {
    try {
      setIsStarting(true);
      if (isConnected()) {
        await StopProxy();
        setIsConnected(false);
      } else {
        await StartProxy();
        setIsConnected(true);
      }
    } catch (error) {
      console.error("Failed to toggle proxy:", error);
      setIsConnected(false);
    } finally {
      setIsStarting(false);
    }
  };

  const [copiedDelay, setCopiedDelay] = createSignal(false);

  const handleCopyServerAddress = () => {
    const textarea = document.createElement("textarea");
    textarea.value = serverAddress()!;
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);

    setCopiedDelay(true);
    setTimeout(() => setCopiedDelay(false), 3000);
  };

  return (
    <div class="h-screen w-full flex items-center justify-center relative">
      <div class="absolute bottom-3 right-3">
        <button class="btn btn-xs btn-ghost" onClick={props.OnClickSettings}>
          Настройки
        </button>
      </div>

      <Show when={isConnected()}>
        <div class="absolute left-1/2 top-8 -translate-x-1/2">
          <button
            disabled={copiedDelay()}
            class="shadow-sm text-base-content/60 hover:text-base-content shadow-base-content/10 px-5 py-2 cursor-pointer flex items-center gap-2 bg-base-300 rounded-lg font-bold text-xs hover:opacity-90 transition-all duration-500"
            onClick={handleCopyServerAddress}
          >
            <span>{copiedDelay() ? "Скопировано" : serverAddress()}</span>
            <FaRegularCopy size={12} />
          </button>
        </div>
      </Show>

      <div
        title={isConnected() ? "Остановить прокси" : "Запустить прокси"}
        class="loader-wrapper"
        classList={{ "loader-wrapper-stopped": !isConnected() }}
        onClick={handleToggleProxy}
      >
        <For
          each={
            isStarting()
              ? startingText.split("")
              : isConnected()
              ? connectedText.split("")
              : notConnectedText.split("")
          }
        >
          {(letter) => (
            <span
              class="loader-letter"
              classList={{ "loader-letter": isStarting() || isConnected() }}
            >
              {letter}
            </span>
          )}
        </For>

        <div class="loader"></div>
      </div>
    </div>
  );
};
