import React, { createContext, useContext, useEffect, useState } from 'react';
import { UserManager, WebStorageStateStore } from 'oidc-client-ts';
import { oidcConfig } from './authConfig';

const AuthContext = createContext();

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

export const AuthProvider = ({ children }) => {
  const [userManager] = useState(() => {
    return new UserManager({
      ...oidcConfig,
      userStore: new WebStorageStateStore({ store: window.localStorage }),
    });
  });

  const [user, setUser] = useState(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    // Load user on mount
    userManager.getUser().then((user) => {
      if (user && !user.expired) {
        setUser(user);
      }
      setIsLoading(false);
    });

    // Listen for user loaded event
    const handleUserLoaded = (user) => {
      setUser(user);
      setIsLoading(false);
    };

    // Listen for user unloaded event
    const handleUserUnloaded = () => {
      setUser(null);
    };

    userManager.events.addUserLoaded(handleUserLoaded);
    userManager.events.addUserUnloaded(handleUserUnloaded);

    return () => {
      userManager.events.removeUserLoaded(handleUserLoaded);
      userManager.events.removeUserUnloaded(handleUserUnloaded);
    };
  }, [userManager]);

  const login = () => {
    return userManager.signinRedirect();
  };

  const logout = () => {
    setUser(null);
    return userManager.signoutRedirect();
  };

  const handleCallback = () => {
    return userManager.signinRedirectCallback();
  };

  const value = {
    user,
    isLoading,
    isAuthenticated: !!user && !user.expired,
    login,
    logout,
    handleCallback,
    userManager,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

